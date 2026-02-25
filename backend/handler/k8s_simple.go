package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	context2 "context"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"grpc-inspector/auth"
	"grpc-inspector/grpcclient"
)

// ── Kubeconfig store ──────────────────────────────────────────────────────────

type uploadedKubeconfig struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Contexts       []string `json:"contexts"`
	CurrentContext string   `json:"currentContext"`
	UploadedAt     string   `json:"uploadedAt"`
	ownerID        string // never exported — ownership check
	rawBytes       []byte // never exported — written to disk only when kubectl runs
}

type activeForward struct {
	ID           string `json:"id"`
	ConfigID     string `json:"configId"`
	Namespace    string `json:"namespace"`
	Service      string `json:"service"`
	LocalPort    int    `json:"localPort"`
	RemotePort   int    `json:"remotePort"`
	Address      string `json:"address"`
	ConnectionID string `json:"connectionId"`
	cmd          *exec.Cmd
	cancel       context2.CancelFunc
}

// K8sSimpleHandler — kubeconfig parse + kubectl subprocess port-forwarding
type K8sSimpleHandler struct {
	mu       sync.RWMutex
	configs  map[string]*uploadedKubeconfig
	forwards map[string]*activeForward
	db       interface {
		CheckFeatureAccess(userID, feature string) error
	}
}

func NewK8sSimpleHandler(db interface {
	CheckFeatureAccess(userID, feature string) error
}) *K8sSimpleHandler {
	return &K8sSimpleHandler{
		configs:  make(map[string]*uploadedKubeconfig),
		forwards: make(map[string]*activeForward),
		db:       db,
	}
}

// writeTempKubeconfig writes kubeconfig bytes to a private temp directory
// (mode 0700) as a file named "config" with mode 0600.
// The returned cleanup func removes the entire temp directory — callers MUST
// call it when kubectl no longer needs the file.
// If data is empty, it returns ("", no-op, nil) so kubectl falls back to
// the default ~/.kube/config without a --kubeconfig flag.
func writeTempKubeconfig(data []byte) (path string, cleanup func(), err error) {
	if len(data) == 0 {
		return "", func() {}, nil
	}
	dir, err := os.MkdirTemp("", "kc-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp dir: %w", err)
	}
	// Ensure directory is accessible only by the current process owner.
	if err := os.Chmod(dir, 0700); err != nil {
		os.RemoveAll(dir)
		return "", func() {}, fmt.Errorf("chmod temp dir: %w", err)
	}
	path = filepath.Join(dir, "config")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		os.RemoveAll(dir)
		return "", func() {}, fmt.Errorf("create kubeconfig file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.RemoveAll(dir)
		return "", func() {}, fmt.Errorf("write kubeconfig: %w", err)
	}
	f.Close()
	return path, func() { os.RemoveAll(dir) }, nil
}

// validateContext ensures the requested context name is in the kubeconfig's
// declared context list — prevents passing arbitrary strings to kubectl --context.
// When allowed is nil (default ~/.kube/config path), any context is permitted
// since kubectl will validate it against the file itself.
func validateContext(ctxName string, allowed []string) bool {
	if ctxName == "" {
		return true // empty → kubectl uses current-context from file
	}
	if len(allowed) == 0 {
		return true // default kubeconfig — let kubectl validate the context
	}
	for _, c := range allowed {
		if c == ctxName {
			return true
		}
	}
	return false
}

// parseKubeconfigContexts extracts context names and current-context via line parsing
func parseKubeconfigContexts(data []byte) (contexts []string, currentContext string) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inContexts := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "contexts:" {
			inContexts = true
			continue
		}
		if inContexts && len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '-' {
			inContexts = false
		}
		if inContexts && strings.HasPrefix(trimmed, "- name:") {
			name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")), `"'`)
			if name != "" {
				contexts = append(contexts, name)
			}
		}
		if strings.HasPrefix(trimmed, "current-context:") {
			currentContext = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "current-context:")), `"'`)
		}
	}
	return
}

// ── Kubeconfig endpoints ──────────────────────────────────────────────────────

// POST /api/k8s/kubeconfigs
func (h *K8sSimpleHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Feature gate — K8s integration requires Professional+ plan
	if h.db != nil {
		if err := h.db.CheckFeatureAccess(claims.UserID, "k8s"); err != nil {
			jsonError(w, err.Error(), http.StatusPaymentRequired)
			return
		}
	}

	var data []byte
	var name string

	ct := r.Header.Get("Content-Type")
	log.Printf("[k8s] Upload called, Content-Type: %q", ct)

	if strings.Contains(ct, "multipart") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			jsonError(w, "parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		name = r.FormValue("name")
		f, _, err := r.FormFile("file")
		if err != nil {
			jsonError(w, "read file: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		data, _ = io.ReadAll(io.LimitReader(f, 1<<20))
	} else {
		name = r.Header.Get("X-Config-Name")
		data, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
	}

	if len(data) == 0 {
		jsonError(w, "empty body", http.StatusBadRequest)
		return
	}

	contexts, currentContext := parseKubeconfigContexts(data)
	log.Printf("[k8s] Parsed %d contexts, current: %q", len(contexts), currentContext)
	if len(contexts) == 0 {
		jsonError(w, "no contexts found — is this a valid kubeconfig?", http.StatusBadRequest)
		return
	}

	if name == "" {
		name = "kubeconfig-" + time.Now().Format("0102-1504")
	}

	// Raw bytes are kept only in memory — no disk write until kubectl actually runs.
	id := generateID()
	entry := &uploadedKubeconfig{
		ID:             id,
		Name:           name,
		Contexts:       contexts,
		CurrentContext: currentContext,
		UploadedAt:     time.Now().Format(time.RFC3339),
		ownerID:        claims.UserID,
		rawBytes:       data,
	}

	h.mu.Lock()
	h.configs[id] = entry
	h.mu.Unlock()

	jsonOK(w, entry)
}

// GET /api/k8s/kubeconfigs
func (h *K8sSimpleHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*uploadedKubeconfig, 0)
	for _, c := range h.configs {
		if c.ownerID == claims.UserID {
			list = append(list, c)
		}
	}
	jsonOK(w, map[string]interface{}{"configs": list})
}

// DELETE /api/k8s/kubeconfigs/{id}
func (h *K8sSimpleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := mux.Vars(r)["id"]
	h.mu.Lock()
	cfg, ok := h.configs[id]
	if ok {
		if cfg.ownerID != claims.UserID {
			h.mu.Unlock()
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		// Zero out sensitive bytes before GC
		for i := range cfg.rawBytes {
			cfg.rawBytes[i] = 0
		}
		delete(h.configs, id)
	}
	h.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/k8s/kubeconfigs/{id}/context
func (h *K8sSimpleHandler) SwitchContext(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := mux.Vars(r)["id"]
	h.mu.Lock()
	cfg, ok := h.configs[id]
	if ok && cfg.ownerID != claims.UserID {
		h.mu.Unlock()
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}
	h.mu.Unlock()
	if !ok {
		jsonError(w, "config not found", http.StatusNotFound)
		return
	}
	var req struct {
		Context string `json:"context"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Context != "" {
		if !validateContext(req.Context, cfg.Contexts) {
			jsonError(w, "invalid context", http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		cfg.CurrentContext = req.Context
		h.mu.Unlock()
	}
	jsonOK(w, cfg)
}

// GET /api/k8s/namespaces?configId=xxx&context=yyy — runs: kubectl get namespaces
func (h *K8sSimpleHandler) Namespaces(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rawBytes, allowedContexts, storedCtx, err := h.resolveKubeconfig(claims.UserID, r.URL.Query().Get("configId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctxName := r.URL.Query().Get("context")
	if ctxName == "" {
		ctxName = storedCtx
	}
	if !validateContext(ctxName, allowedContexts) {
		jsonError(w, "invalid context", http.StatusBadRequest)
		return
	}

	// Write temp file — deleted as soon as kubectl exits
	tmpPath, cleanup, err := writeTempKubeconfig(rawBytes)
	if err != nil {
		jsonError(w, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	args := []string{"get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}"}
	if tmpPath != "" {
		args = append(args, "--kubeconfig", tmpPath)
	}
	if ctxName != "" {
		args = append(args, "--context", ctxName)
	}

	ctx, cancel := context2.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		stderr := ""
		if errors.As(err, &exitErr) {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		msg := "kubectl get namespaces failed"
		if stderr != "" {
			msg += ": " + stderr
		} else {
			msg += ": " + err.Error()
		}
		jsonError(w, msg, http.StatusBadGateway)
		return
	}
	namespaces := strings.Fields(string(out))
	if len(namespaces) == 0 {
		namespaces = []string{}
	}
	jsonOK(w, map[string]interface{}{"namespaces": namespaces})
}

// GET /api/k8s/services?configId=xxx&context=yyy&namespace=zzz
func (h *K8sSimpleHandler) Services(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rawBytes, allowedContexts, storedCtx, err := h.resolveKubeconfig(claims.UserID, r.URL.Query().Get("configId"))
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctxName := r.URL.Query().Get("context")
	if ctxName == "" {
		ctxName = storedCtx
	}
	if !validateContext(ctxName, allowedContexts) {
		jsonError(w, "invalid context", http.StatusBadRequest)
		return
	}

	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}

	tmpPath, cleanup, err := writeTempKubeconfig(rawBytes)
	if err != nil {
		jsonError(w, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()

	args := []string{"get", "services", "-n", ns,
		"-o", `jsonpath={range .items[*]}{.metadata.name}{"\t"}{range .spec.ports[*]}{.name}{":"}{.port}{","}{end}{"\n"}{end}`}
	if tmpPath != "" {
		args = append(args, "--kubeconfig", tmpPath)
	}
	if ctxName != "" {
		args = append(args, "--context", ctxName)
	}

	ctx, cancel := context2.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		stderr := ""
		if errors.As(err, &exitErr) {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		msg := "kubectl get services failed"
		if stderr != "" {
			msg += ": " + stderr
		} else {
			msg += ": " + err.Error()
		}
		jsonError(w, msg, http.StatusBadGateway)
		return
	}

	type K8sSvcPort struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}
	type K8sSvc struct {
		Name      string       `json:"name"`
		Namespace string       `json:"namespace"`
		Ports     []K8sSvcPort `json:"ports"`
	}

	var svcs []K8sSvc
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 || parts[0] == "" {
			continue
		}
		svc := K8sSvc{Name: parts[0], Namespace: ns}
		for _, p := range strings.Split(strings.TrimRight(parts[1], ","), ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			np := strings.SplitN(p, ":", 2)
			port := 0
			if len(np) == 2 {
				fmt.Sscanf(np[1], "%d", &port)
			}
			svc.Ports = append(svc.Ports, K8sSvcPort{Name: np[0], Port: port})
		}
		svcs = append(svcs, svc)
	}
	jsonOK(w, map[string]interface{}{"services": svcs})
}

// GET /api/k8s/forwards — list active port-forwards
func (h *K8sSimpleHandler) ListForwards(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*activeForward, 0, len(h.forwards))
	for _, f := range h.forwards {
		list = append(list, f)
	}
	jsonOK(w, map[string]interface{}{"forwards": list})
}

// DELETE /api/k8s/forwards/{id}
func (h *K8sSimpleHandler) StopForward(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	h.mu.Lock()
	fwd, ok := h.forwards[id]
	if ok {
		fwd.cancel()
		delete(h.forwards, id)
	}
	h.mu.Unlock()
	if !ok {
		jsonError(w, "forward not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/k8s/connect
// Body: { configId, namespace, service, port, tls, insecure, metadata }
func (h *K8sSimpleHandler) ForwardAndConnect(connHandler *ConnectionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r.Context())
		if claims == nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req struct {
			ConfigID  string            `json:"configId"`
			Context   string            `json:"context"`
			Namespace string            `json:"namespace"`
			Service   string            `json:"service"`
			Port      int               `json:"port"`
			Name      string            `json:"name"`
			TLS       bool              `json:"tls"`
			Insecure  bool              `json:"insecure"`
			Metadata  map[string]string `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Namespace == "" || req.Service == "" || req.Port == 0 {
			jsonError(w, "namespace, service and port are required", http.StatusBadRequest)
			return
		}

		rawBytes, allowedContexts, storedCtx, err := h.resolveKubeconfig(claims.UserID, req.ConfigID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctxName := storedCtx
		if req.Context != "" {
			ctxName = req.Context
		}
		if !validateContext(ctxName, allowedContexts) {
			jsonError(w, "invalid context", http.StatusBadRequest)
			return
		}

		// Write temp file — cleaned up when kubectl port-forward process exits
		tmpPath, cleanupFile, err := writeTempKubeconfig(rawBytes)
		if err != nil {
			jsonError(w, "internal error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Find a free local port
		localPort, err := freePort()
		if err != nil {
			cleanupFile()
			jsonError(w, "no free port: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Build kubectl port-forward command
		args := []string{
			"port-forward",
			fmt.Sprintf("svc/%s", req.Service),
			fmt.Sprintf("%d:%d", localPort, req.Port),
			"-n", req.Namespace,
		}
		if tmpPath != "" {
			args = append(args, "--kubeconfig", tmpPath)
		}
		if ctxName != "" {
			args = append(args, "--context", ctxName)
		}

		ctx, cancel := context2.WithCancel(context2.Background())
		cmd := exec.CommandContext(ctx, "kubectl", args...)
		cmd.Stdout = io.Discard

		// kubectl writes "Forwarding from ..." to stderr
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			cancel()
			cleanupFile()
			jsonError(w, "stderr pipe: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := cmd.Start(); err != nil {
			cancel()
			cleanupFile()
			jsonError(w, "kubectl start: "+err.Error(), http.StatusBadGateway)
			return
		}

		log.Printf("[k8s] port-forward started: kubectl %s", strings.Join(args, " "))

		// Drain stderr to log and collect error messages
		var stderrLines []string
		var stderrMu sync.Mutex
		go func() {
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				line := scanner.Text()
				log.Printf("[k8s pf stderr] %s", line)
				stderrMu.Lock()
				stderrLines = append(stderrLines, line)
				stderrMu.Unlock()
			}
		}()

		// Watch for early process exit
		doneCh := make(chan error, 1)
		go func() { doneCh <- cmd.Wait() }()

		// Poll TCP port until open (max 10s)
		readyCh := make(chan error, 1)
		go func() {
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 300*time.Millisecond)
				if err == nil {
					conn.Close()
					readyCh <- nil
					return
				}
				time.Sleep(200 * time.Millisecond)
			}
			stderrMu.Lock()
			msg := strings.Join(stderrLines, "; ")
			stderrMu.Unlock()
			if msg == "" {
				msg = "port did not open within 10s"
			}
			readyCh <- fmt.Errorf("%s", msg)
		}()

		select {
		case err := <-readyCh:
			if err != nil {
				cancel()
				cmd.Wait()
				cleanupFile()
				jsonError(w, "port-forward failed: "+err.Error(), http.StatusBadGateway)
				return
			}
		case err := <-doneCh:
			cancel()
			cleanupFile()
			stderrMu.Lock()
			msg := strings.Join(stderrLines, "; ")
			stderrMu.Unlock()
			if err != nil && msg == "" {
				msg = err.Error()
			}
			jsonError(w, "port-forward failed: "+msg, http.StatusBadGateway)
			return
		}

		// Connect gRPC
		fwdID := generateID()
		address := fmt.Sprintf("localhost:%d", localPort)

		mc, connErr := connHandler.Pool().Connect(fwdID, grpcclient.ConnectOptions{
			Address:  address,
			TLS:      req.TLS,
			Insecure: req.Insecure,
			Metadata: req.Metadata,
		})
		if connErr != nil {
			cancel()
			cleanupFile()
			jsonError(w, "grpc connect: "+connErr.Error(), http.StatusBadGateway)
			return
		}

		name := req.Name
		if name == "" {
			name = fmt.Sprintf("%s/%s", req.Namespace, req.Service)
		}
		connMeta[fwdID] = &connectionMeta{Name: name, CreatedAt: time.Now()}

		fwd := &activeForward{
			ID:           fwdID,
			ConfigID:     req.ConfigID,
			Namespace:    req.Namespace,
			Service:      req.Service,
			LocalPort:    localPort,
			RemotePort:   req.Port,
			Address:      address,
			ConnectionID: mc.ID,
			cmd:          cmd,
			cancel:       cancel,
		}
		h.mu.Lock()
		h.forwards[fwdID] = fwd
		h.mu.Unlock()

		// Watch for kubectl dying — clean up connection AND temp kubeconfig file
		go func() {
			<-doneCh
			log.Printf("[k8s] port-forward ended for %s/%s", req.Namespace, req.Service)
			cleanupFile() // temp kubeconfig no longer needed
			h.mu.Lock()
			delete(h.forwards, fwdID)
			h.mu.Unlock()
		}()

		jsonOK(w, map[string]interface{}{
			"forward":    fwd,
			"connection": toResponse(mc, name),
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// resolveKubeconfig returns the raw kubeconfig bytes and context info for the
// given configID, after verifying the requesting user owns the config.
// Returns empty bytes and no error when configID is "" or "default" — kubectl
// will fall back to ~/.kube/config in that case.
func (h *K8sSimpleHandler) resolveKubeconfig(userID, configID string) (rawBytes []byte, allowedContexts []string, ctxName string, err error) {
	if configID == "" || configID == "default" {
		return nil, nil, "", nil // kubectl uses ~/.kube/config
	}
	h.mu.RLock()
	cfg, ok := h.configs[configID]
	h.mu.RUnlock()
	if !ok {
		return nil, nil, "", fmt.Errorf("config %q not found", configID)
	}
	if cfg.ownerID != userID {
		return nil, nil, "", fmt.Errorf("config %q not found", configID) // same message — don't reveal existence
	}
	return cfg.rawBytes, cfg.Contexts, cfg.CurrentContext, nil
}

func freePort() (int, error) {
	for i := 0; i < 10; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		port := ln.Addr().(*net.TCPAddr).Port
		ln.Close()
		time.Sleep(50 * time.Millisecond)
		ln2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		ln2.Close()
		return port, nil
	}
	return 0, fmt.Errorf("could not find a free port after 10 attempts")
}

// kubeconfigDir returns the default kubeconfig path
func kubeconfigDir() string {
	if k := os.Getenv("KUBECONFIG"); k != "" {
		return filepath.Dir(k)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube")
}
