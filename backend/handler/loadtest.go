package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"grpc-inspector/auth"
	"grpc-inspector/db"
	"grpc-inspector/grpcclient"
)

type LoadTestHandler struct {
	db          *db.DB
	connHandler *ConnectionHandler
}

func NewLoadTestHandler(database *db.DB, connHandler *ConnectionHandler) *LoadTestHandler {
	return &LoadTestHandler{db: database, connHandler: connHandler}
}

type LoadTestRequest struct {
	Service     string            `json:"service"`
	Method      string            `json:"method"`
	Payload     json.RawMessage   `json:"payload"`
	ReqMetadata map[string]string `json:"metadata"`
	Concurrency int               `json:"concurrency"`
	TotalCalls  int               `json:"totalCalls"`
	WarmupCalls int               `json:"warmupCalls"`
}

type LoadTestResult struct {
	Service         string  `json:"service"`
	Method          string  `json:"method"`
	Concurrency     int     `json:"concurrency"`
	TotalCalls      int     `json:"totalCalls"`
	TotalDurationMs int64   `json:"totalDurationMs"`
	Throughput      float64 `json:"throughput"`
	SuccessCount    int64   `json:"successCount"`
	ErrorCount      int64   `json:"errorCount"`
	ErrorRate       float64 `json:"errorRate"`
	LatencyMin      float64 `json:"latencyMin"`
	LatencyMean     float64 `json:"latencyMean"`
	LatencyP50      float64 `json:"latencyP50"`
	LatencyP90      float64 `json:"latencyP90"`
	LatencyP95      float64 `json:"latencyP95"`
	LatencyP99      float64 `json:"latencyP99"`
	LatencyMax      float64 `json:"latencyMax"`
	LatencyStdDev   float64 `json:"latencyStdDev"`
	Buckets         []LoadBucket    `json:"buckets"`
	Errors          map[string]int64 `json:"errors"`
}

type LoadBucket struct {
	Second     int     `json:"second"`
	Requests   int64   `json:"requests"`
	Errors     int64   `json:"errors"`
	LatencyP50 float64 `json:"latencyP50"`
	LatencyP99 float64 `json:"latencyP99"`
	Throughput float64 `json:"throughput"`
}

type ltCallResult struct {
	durationMs float64
	errStr     string
	startedAt  time.Time
}

// POST /api/connections/{id}/loadtest
func (h *LoadTestHandler) Run(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.db.CheckFeatureAccess(claims.UserID, "load_testing"); err != nil {
		jsonPaymentRequired(w, err)
		return
	}

	connID := mux.Vars(r)["id"]
	mc, err := h.connHandler.Pool().Get(connID)
	if err != nil {
		jsonError(w, "connection not found", http.StatusNotFound)
		return
	}

	var req LoadTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Clamp values
	req.Concurrency = clampInt(req.Concurrency, 1, 50)
	req.TotalCalls  = clampInt(req.TotalCalls, 1, 5000)
	req.WarmupCalls = clampInt(req.WarmupCalls, 0, req.TotalCalls/2)

	log.Printf("[loadtest] conn=%s %s/%s concurrency=%d total=%d",
		connID, req.Service, req.Method, req.Concurrency, req.TotalCalls)

	// Resolve method descriptor
	ctxReflect, cancelReflect := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancelReflect()

	svcDesc, err := resolveServiceDescriptor(ctxReflect, connID, req.Service, mc)
	if err != nil {
		jsonError(w, "reflect: "+err.Error(), http.StatusBadGateway)
		return
	}
	var methodDesc *grpcclient.MethodDescriptor
	for _, m := range svcDesc.Methods {
		if m.Name == req.Method {
			methodDesc = m
			break
		}
	}
	if methodDesc == nil {
		jsonError(w, fmt.Sprintf("method %q not found", req.Method), http.StatusBadRequest)
		return
	}
	if methodDesc.ClientStreaming || methodDesc.ServerStreaming {
		jsonError(w, "load testing supports unary methods only", http.StatusBadRequest)
		return
	}

	// Validate payload once before spawning workers
	valCtx, valCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer valCancel()
	_, err = buildMsg(valCtx, connID, grpcclient.TrimLeadingDot(methodDesc.InputType), req.Payload, mc)
	if err != nil {
		jsonError(w, "invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	inputType  := grpcclient.TrimLeadingDot(methodDesc.InputType)
	outputType := grpcclient.TrimLeadingDot(methodDesc.OutputType)
	fullMethod := fmt.Sprintf("/%s/%s", ltTrimDot(svcDesc.Name), req.Method)
	totalWithWarmup := req.TotalCalls + req.WarmupCalls

	// Work queue
	work := make(chan struct{}, totalWithWarmup)
	for i := 0; i < totalWithWarmup; i++ {
		work <- struct{}{}
	}
	close(work)

	var (
		allResults []ltCallResult
		mu         sync.Mutex
		wg         sync.WaitGroup
	)

	testStart := time.Now()

	for w := 0; w < req.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range work {
				callCtx, callCancel := context.WithTimeout(context.Background(), 15*time.Second)

				md := metadata.New(mc.Options.Metadata)
				for k, v := range req.ReqMetadata {
					md.Set(k, v)
				}
				callCtx = metadata.NewOutgoingContext(callCtx, md)

				inputMsg, buildErr := buildMsg(callCtx, connID, inputType, req.Payload, mc)
				outputMsg, _ := buildMsg(callCtx, connID, outputType, nil, mc)

				started := time.Now()
				var callErr error
				if buildErr != nil {
					callErr = buildErr
				} else {
					callErr = mc.Get().Invoke(callCtx, fullMethod,
						inputMsg, outputMsg, grpc.EmptyCallOption{})
				}
				elapsed := float64(time.Since(started).Microseconds()) / 1000.0
				callCancel()

				cr := ltCallResult{durationMs: elapsed, startedAt: started}
				if callErr != nil {
					s := callErr.Error()
					if len(s) > 80 {
						s = s[:80]
					}
					cr.errStr = s
				}
				mu.Lock()
				allResults = append(allResults, cr)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(testStart)

	// Discard warmup
	measured := allResults
	if req.WarmupCalls > 0 && len(allResults) > req.WarmupCalls {
		measured = allResults[req.WarmupCalls:]
	}

	// Aggregate
	latencies := make([]float64, 0, len(measured))
	errMap := make(map[string]int64)
	var successCount, errorCount int64

	for _, cr := range measured {
		if cr.errStr != "" {
			errorCount++
			errMap[cr.errStr]++
		} else {
			successCount++
			latencies = append(latencies, cr.durationMs)
		}
	}
	sort.Float64s(latencies)

	res := &LoadTestResult{
		Service:         req.Service,
		Method:          req.Method,
		Concurrency:     req.Concurrency,
		TotalCalls:      len(measured),
		TotalDurationMs: totalDuration.Milliseconds(),
		SuccessCount:    successCount,
		ErrorCount:      errorCount,
		Errors:          errMap,
	}
	if len(measured) > 0 {
		res.ErrorRate  = float64(errorCount) / float64(len(measured))
		res.Throughput = float64(len(measured)) / totalDuration.Seconds()
	}
	if len(latencies) > 0 {
		res.LatencyMin    = latencies[0]
		res.LatencyMax    = latencies[len(latencies)-1]
		res.LatencyMean   = ltMean(latencies)
		res.LatencyP50    = ltPercentile(latencies, 50)
		res.LatencyP90    = ltPercentile(latencies, 90)
		res.LatencyP95    = ltPercentile(latencies, 95)
		res.LatencyP99    = ltPercentile(latencies, 99)
		res.LatencyStdDev = ltStdDev(latencies)
	}
	res.Buckets = ltBuckets(measured, testStart)

	_ = h.db.IncrementUsage(claims.UserID, len(measured))
	jsonOK(w, res)
}

// ── Stats helpers ─────────────────────────────────────────────────────────────

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func ltTrimDot(s string) string {
	if len(s) > 0 && s[0] == '.' {
		return s[1:]
	}
	return s
}

func ltPercentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx   := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func ltMean(vals []float64) float64 {
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func ltStdDev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := ltMean(vals)
	var variance float64
	for _, v := range vals {
		d := v - m
		variance += d * d
	}
	variance /= float64(len(vals) - 1)
	return math.Sqrt(variance)
}

func ltBuckets(results []ltCallResult, testStart time.Time) []LoadBucket {
	if len(results) == 0 {
		return nil
	}
	var maxSec int
	for _, r := range results {
		if s := int(r.startedAt.Sub(testStart).Seconds()); s > maxSec {
			maxSec = s
		}
	}
	buckets     := make([]LoadBucket, maxSec+1)
	bucketLats  := make([][]float64, maxSec+1)
	for i := range buckets {
		buckets[i].Second = i
	}
	for _, r := range results {
		s := int(r.startedAt.Sub(testStart).Seconds())
		if s >= len(buckets) {
			continue
		}
		buckets[s].Requests++
		if r.errStr != "" {
			buckets[s].Errors++
		} else {
			bucketLats[s] = append(bucketLats[s], r.durationMs)
		}
	}
	for i := range buckets {
		lats := bucketLats[i]
		sort.Float64s(lats)
		if len(lats) > 0 {
			buckets[i].LatencyP50 = ltPercentile(lats, 50)
			buckets[i].LatencyP99 = ltPercentile(lats, 99)
		}
		buckets[i].Throughput = float64(buckets[i].Requests)
	}
	return buckets
}
