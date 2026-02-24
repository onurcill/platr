package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"grpc-inspector/grpcclient"
)

// ReflectionHandler serves gRPC server reflection results over HTTP.
type ReflectionHandler struct {
	connHandler *ConnectionHandler
}

func NewReflectionHandler(connHandler *ConnectionHandler) *ReflectionHandler {
	return &ReflectionHandler{connHandler: connHandler}
}

// ListServices returns services from reflection AND uploaded proto cache.
// GET /api/connections/{id}/reflect/services
func (h *ReflectionHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	mc, err := h.getConn(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Always include proto-cached services (uploaded .proto files)
	cachedSvcs := GetCachedServices(mc.ID)
	cachedNames := make([]string, 0, len(cachedSvcs))
	for _, s := range cachedSvcs {
		cachedNames = append(cachedNames, s.Name)
	}

	// Try reflection — merge with cached if successful
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	rc := grpcclient.NewReflectionClient(mc.Get())
	reflectedNames, reflectErr := rc.ListServices(ctx)

	if reflectErr != nil && len(cachedNames) == 0 {
		// Neither reflection nor cache available
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "reflection failed: " + reflectErr.Error(),
			"suggestion": "Enable gRPC server reflection or upload a .proto descriptor file",
			"code":       "REFLECTION_UNAVAILABLE",
		})
		return
	}

	// Merge: cached + reflected (deduplicated)
	seen := map[string]bool{}
	all := []string{}
	for _, n := range cachedNames {
		if !seen[n] { seen[n] = true; all = append(all, n) }
	}
	for _, n := range reflectedNames {
		if !seen[n] { seen[n] = true; all = append(all, n) }
	}

	jsonOK(w, map[string]interface{}{
		"connectionId": mc.ID,
		"services":     all,
		"fromCache":    len(cachedNames) > 0,
		"fromReflection": reflectErr == nil,
	})
}

// DescribeService returns methods + message schemas for a service.
// GET /api/connections/{id}/reflect/service/{service}
func (h *ReflectionHandler) DescribeService(w http.ResponseWriter, r *http.Request) {
	mc, err := h.getConn(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	serviceName := mux.Vars(r)["service"]

	// Check proto cache first
	cached := GetCachedServices(mc.ID)
	for _, svc := range cached {
		if svc.Name == serviceName {
			jsonOK(w, svc)
			return
		}
	}

	// Fall back to reflection
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	rc := grpcclient.NewReflectionClient(mc.Get())
	desc, err := rc.DescribeService(ctx, serviceName)
	if err != nil {
		jsonError(w, "describe service failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	jsonOK(w, desc)
}

// DescribeMethod returns schema details for a single method.
// GET /api/connections/{id}/reflect/method/{service}/{method}
func (h *ReflectionHandler) DescribeMethod(w http.ResponseWriter, r *http.Request) {
	mc, err := h.getConn(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	vars := mux.Vars(r)
	serviceName := vars["service"]
	methodName := vars["method"]

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	rc := grpcclient.NewReflectionClient(mc.Get())
	desc, err := rc.DescribeService(ctx, serviceName)
	if err != nil {
		jsonError(w, "describe failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	for _, m := range desc.Methods {
		if m.Name == methodName {
			jsonOK(w, m)
			return
		}
	}

	jsonError(w, "method not found: "+methodName, http.StatusNotFound)
}

func (h *ReflectionHandler) getConn(r *http.Request) (*grpcclient.ManagedConn, error) {
	id := mux.Vars(r)["id"]
	return h.connHandler.Pool().Get(id)
}
