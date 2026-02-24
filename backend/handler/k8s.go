package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"grpc-inspector/grpcclient"
	"grpc-inspector/k8s"
)

// K8sHandler serves all K8s endpoints.
// Each uploaded kubeconfig gets its own Manager (port-forward pool).
type K8sHandler struct {
	defaultMgr  *k8s.Manager    // from local ~/.kube/config, may be nil
	configStore *k8s.ConfigStore
	managers    map[string]*k8s.Manager // configID -> Manager cache
}

func NewK8sHandler() *K8sHandler {
	h := &K8sHandler{
		configStore: k8s.NewConfigStore(),
		managers:    make(map[string]*k8s.Manager),
	}
	// Try local kubeconfig — non-fatal if missing
	if mgr, err := k8s.NewManager(); err == nil {
		h.defaultMgr = mgr
	}
	return h
}

// ── Status ────────────────────────────────────────────────────────────────────

// GET /api/k8s/status
func (h *K8sHandler) Status(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]interface{}{
		"available":      true,
		"hasDefault":     h.defaultMgr != nil,
		"uploadedCount":  len(h.configStore.List()),
	})
}

// ── Kubeconfig management ─────────────────────────────────────────────────────

// POST /api/k8s/kubeconfigs
// Accepts multipart (field: file + name) OR raw body with X-Config-Name header.
func (h *K8sHandler) UploadKubeconfig(w http.ResponseWriter, r *http.Request) {
	var (
		name string
		data []byte
		err  error
	)

	ct := r.Header.Get("Content-Type")
	if len(ct) >= 9 && ct[:9] == "multipart" {
		if err = r.ParseMultipartForm(1 << 20); err != nil {
			jsonError(w, "parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		name = r.FormValue("name")
		f, _, ferr := r.FormFile("file")
		if ferr != nil {
			jsonError(w, "read file field: "+ferr.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		data, err = io.ReadAll(io.LimitReader(f, 1<<20))
	} else {
		// Raw body (text/plain or application/octet-stream)
		name = r.Header.Get("X-Config-Name")
		data, err = io.ReadAll(io.LimitReader(r.Body, 1<<20))
	}

	if err != nil || len(data) == 0 {
		jsonError(w, "empty or unreadable body", http.StatusBadRequest)
		return
	}
	if name == "" {
		name = "kubeconfig-" + time.Now().Format("0102-1504")
	}

	entry, addErr := h.configStore.Add(name, data)
	if addErr != nil {
		jsonError(w, addErr.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, entry)
}

// GET /api/k8s/kubeconfigs
func (h *K8sHandler) ListKubeconfigs(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]interface{}{
		"configs":    h.configStore.List(),
		"hasDefault": h.defaultMgr != nil,
	})
}

// DELETE /api/k8s/kubeconfigs/{id}
func (h *K8sHandler) DeleteKubeconfig(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.configStore.Remove(id); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	delete(h.managers, id)
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/k8s/kubeconfigs/{id}/context
// Body: { "context": "my-context" }
func (h *K8sHandler) SwitchContext(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req struct {
		Context string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Context == "" {
		jsonError(w, "context is required", http.StatusBadRequest)
		return
	}
	entry, err := h.configStore.SwitchContext(id, req.Context)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	// Invalidate cached manager so it's rebuilt on next use
	delete(h.managers, id)
	jsonOK(w, entry)
}

// ── Discovery ─────────────────────────────────────────────────────────────────

// GET /api/k8s/namespaces?configId=<id>
func (h *K8sHandler) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	mgr, err := h.resolveManager(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	names, err := mgr.ListNamespaces(ctx)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, map[string]interface{}{"namespaces": names})
}

// GET /api/k8s/services?configId=<id>&namespace=foo
func (h *K8sHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	mgr, err := h.resolveManager(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	svcs, err := mgr.ListServices(ctx, r.URL.Query()["namespace"])
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, map[string]interface{}{"services": svcs})
}

// ── Port-forwards ─────────────────────────────────────────────────────────────

// GET /api/k8s/forwards
func (h *K8sHandler) ListForwards(w http.ResponseWriter, r *http.Request) {
	all := []*k8s.ForwardedService{}
	if h.defaultMgr != nil {
		all = append(all, h.defaultMgr.ListForwards()...)
	}
	for _, mgr := range h.managers {
		all = append(all, mgr.ListForwards()...)
	}
	jsonOK(w, map[string]interface{}{"forwards": all})
}

// DELETE /api/k8s/forwards/{id}
func (h *K8sHandler) StopForward(w http.ResponseWriter, r *http.Request) {
	fwdID := mux.Vars(r)["id"]
	if h.defaultMgr != nil {
		if err := h.defaultMgr.StopForward(fwdID); err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	for _, mgr := range h.managers {
		if err := mgr.StopForward(fwdID); err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	jsonError(w, "forward not found", http.StatusNotFound)
}

// ── Connect (port-forward + gRPC) ─────────────────────────────────────────────

// POST /api/k8s/connect
// Body: { "configId":"...", "namespace":"...", "service":"...", "port":50051, "name":"...", "tls":false, "insecure":false }
func (h *K8sHandler) ForwardAndConnect(connHandler *ConnectionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ConfigID  string            `json:"configId"`
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

		mgr, err := h.resolveManagerByID(req.ConfigID)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		fwd, err := mgr.Forward(ctx, req.Namespace, req.Service, req.Port)
		if err != nil {
			jsonError(w, "port-forward: "+err.Error(), http.StatusBadGateway)
			return
		}

		name := req.Name
		if name == "" {
			name = req.Namespace + "/" + req.Service
		}

		mc, connErr := connHandler.Pool().Connect(fwd.ID, grpcclient.ConnectOptions{
			Address:  fwd.Address,
			TLS:      req.TLS,
			Insecure: req.Insecure,
			Metadata: req.Metadata,
		})
		if connErr != nil {
			_ = mgr.StopForward(fwd.ID)
			jsonError(w, "grpc connect: "+connErr.Error(), http.StatusBadGateway)
			return
		}

		connMeta[fwd.ID] = &connectionMeta{Name: name, CreatedAt: time.Now()}

		jsonOK(w, map[string]interface{}{
			"forward":    fwd,
			"connection": toResponse(mc, name),
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *K8sHandler) resolveManager(r *http.Request) (*k8s.Manager, error) {
	return h.resolveManagerByID(r.URL.Query().Get("configId"))
}

func (h *K8sHandler) resolveManagerByID(configID string) (*k8s.Manager, error) {
	if configID == "" || configID == "default" {
		if h.defaultMgr == nil {
			return nil, fmt.Errorf("no default kubeconfig — upload one first")
		}
		return h.defaultMgr, nil
	}
	if mgr, ok := h.managers[configID]; ok {
		return mgr, nil
	}
	mgr, err := h.configStore.GetManager(configID)
	if err != nil {
		return nil, err
	}
	h.managers[configID] = mgr
	return mgr, nil
}
