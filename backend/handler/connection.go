package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"grpc-inspector/grpcclient"
)

// ConnectionHandler handles CRUD for gRPC connections.
type ConnectionHandler struct {
	pool *grpcclient.Pool
	// Keep ordered list of ids for listing.
	ids []string
}

func NewConnectionHandler() *ConnectionHandler {
	return &ConnectionHandler{pool: grpcclient.NewPool()}
}

// Pool exposes the connection pool to other handlers.
func (h *ConnectionHandler) Pool() *grpcclient.Pool {
	return h.pool
}

// createRequest is the POST /connections body.
type createRequest struct {
	ID          string            `json:"id"`           // optional; auto-generated if empty
	Name        string            `json:"name"`         // human label
	Address     string            `json:"address"`      // host:port
	TLS         bool              `json:"tls"`
	Insecure    bool              `json:"insecure"`
	Metadata    map[string]string `json:"metadata"`
	DialTimeout int               `json:"dialTimeout"`
}

// connectionResponse is returned in list/create responses.
type connectionResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Address     string            `json:"address"`
	TLS         bool              `json:"tls"`
	State       string            `json:"state"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"createdAt"`
}

// connectionMeta stores extra metadata not held in ManagedConn.
type connectionMeta struct {
	Name      string
	CreatedAt time.Time
}

var connMeta = map[string]*connectionMeta{}

// Create dials a new gRPC connection.
// POST /api/connections
func (h *ConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Address == "" {
		jsonError(w, "address is required", http.StatusBadRequest)
		return
	}

	id := req.ID
	if id == "" {
		id = generateID()
	}

	opts := grpcclient.ConnectOptions{
		Address:     req.Address,
		TLS:         req.TLS,
		Insecure:    req.Insecure,
		Metadata:    req.Metadata,
		DialTimeout: req.DialTimeout,
	}

	mc, err := h.pool.Connect(id, opts)
	if err != nil {
		jsonError(w, "failed to connect: "+err.Error(), http.StatusBadGateway)
		return
	}

	name := req.Name
	if name == "" {
		name = req.Address
	}
	connMeta[id] = &connectionMeta{Name: name, CreatedAt: time.Now()}
	h.ids = append(h.ids, id)

	jsonOK(w, toResponse(mc, name))
}

// List returns all active connections.
// GET /api/connections
func (h *ConnectionHandler) List(w http.ResponseWriter, r *http.Request) {
	conns := h.pool.List()
	out := make([]connectionResponse, 0, len(conns))
	for _, mc := range conns {
		meta := connMeta[mc.ID]
		name := mc.Options.Address
		if meta != nil {
			name = meta.Name
		}
		out = append(out, toResponse(mc, name))
	}
	jsonOK(w, out)
}

// Delete closes and removes a connection.
// DELETE /api/connections/{id}
func (h *ConnectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.pool.Remove(id); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	delete(connMeta, id)
	w.WriteHeader(http.StatusNoContent)
}

// Test checks the current connectivity state.
// POST /api/connections/{id}/test
func (h *ConnectionHandler) Test(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	ok, state, err := h.pool.Test(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]interface{}{
		"id":    id,
		"ok":    ok,
		"state": state,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toResponse(mc *grpcclient.ManagedConn, name string) connectionResponse {
	meta := connMeta[mc.ID]
	createdAt := time.Now()
	if meta != nil {
		createdAt = meta.CreatedAt
	}
	return connectionResponse{
		ID:        mc.ID,
		Name:      name,
		Address:   mc.Options.Address,
		TLS:       mc.Options.TLS,
		State:     mc.State(),
		Metadata:  mc.Options.Metadata,
		CreatedAt: createdAt,
	}
}
