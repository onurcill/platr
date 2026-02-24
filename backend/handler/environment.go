package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"grpc-inspector/auth"
	"grpc-inspector/db"
)

type EnvironmentHandler struct {
	db *db.DB
}

func NewEnvironmentHandler(database *db.DB) *EnvironmentHandler {
	return &EnvironmentHandler{db: database}
}

func (h *EnvironmentHandler) canDo(r *http.Request, workspaceID string, action auth.Action) bool {
	if workspaceID == "" {
		return false
	}
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		return false
	}
	role, _ := h.db.GetMemberRole(workspaceID, claims.UserID)
	return auth.CanString(role, action)
}

// GET /api/environments?workspaceId=xxx
func (h *EnvironmentHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspaceId")
	if wsID == "" {
		jsonOK(w, []*db.Environment{})
		return
	}
	envs, err := h.db.ListEnvironments(wsID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, envs)
}

// POST /api/environments?workspaceId=xxx
func (h *EnvironmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspaceId")
	if wsID == "" {
		jsonError(w, "workspaceId required", http.StatusBadRequest)
		return
	}
	if !h.canDo(r, wsID, auth.ActionEnvironmentManage) {
		jsonError(w, "insufficient permissions: environment:manage", http.StatusForbidden)
		return
	}

	// Enforce environment quota (uses workspace owner's plan)
	if claims := auth.GetClaims(r.Context()); claims != nil {
		ws, _ := h.db.GetWorkspace(wsID)
		ownerID := claims.UserID
		if ws != nil {
			ownerID = ws.OwnerID
		}
		if qErr := h.db.CheckEnvironmentQuota(wsID, ownerID); qErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": qErr.Error(), "quota": qErr, "upgradeRequired": true})
			return
		}
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if req.Color == "" {
		req.Color = "#818cf8"
	}

	env, err := h.db.CreateEnvironment(generateID(), wsID, req.Name, req.Color)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, env)
}

// GET /api/environments/{id}
func (h *EnvironmentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	env, err := h.db.GetEnvironment(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if env == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, env)
}

// PUT /api/environments/{id}
func (h *EnvironmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	env, err := h.db.GetEnvironment(id)
	if err != nil || env == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if !h.canDo(r, env.WorkspaceID, auth.ActionEnvironmentManage) {
		jsonError(w, "insufficient permissions: environment:manage", http.StatusForbidden)
		return
	}

	var req struct {
		Name      string       `json:"name"`
		Color     string       `json:"color"`
		Variables []*db.EnvVar `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	if req.Name != "" {
		if err := h.db.UpdateEnvironment(id, req.Name, req.Color); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if req.Variables != nil {
		if err := h.db.SetEnvVars(id, req.Variables); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	result, err := h.db.GetEnvironment(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, result)
}

// DELETE /api/environments/{id}
func (h *EnvironmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	env, err := h.db.GetEnvironment(id)
	if err != nil || env == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if !h.canDo(r, env.WorkspaceID, auth.ActionEnvironmentManage) {
		jsonError(w, "insufficient permissions: environment:manage", http.StatusForbidden)
		return
	}
	if err := h.db.DeleteEnvironment(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"deleted": id})
}

// POST /api/environments/{id}/duplicate
func (h *EnvironmentHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	src, err := h.db.GetEnvironment(id)
	if err != nil || src == nil {
		jsonError(w, "source not found", http.StatusNotFound)
		return
	}
	if !h.canDo(r, src.WorkspaceID, auth.ActionEnvironmentManage) {
		jsonError(w, "insufficient permissions", http.StatusForbidden)
		return
	}

	newEnv, err := h.db.CreateEnvironment(generateID(), src.WorkspaceID, src.Name+" (copy)", src.Color)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(src.Variables) > 0 {
		if err := h.db.SetEnvVars(newEnv.ID, src.Variables); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	result, _ := h.db.GetEnvironment(newEnv.ID)
	jsonOK(w, result)
}
