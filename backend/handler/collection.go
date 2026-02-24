package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"grpc-inspector/auth"
	"grpc-inspector/db"
)

type CollectionHandler struct {
	db *db.DB
}

func (h *CollectionHandler) canDo(r *http.Request, workspaceID string, action auth.Action) bool {
	if workspaceID == "" {
		return true // no workspace = legacy mode, allow
	}
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		return false
	}
	role, err := h.db.GetMemberRole(workspaceID, claims.UserID)
	if err != nil || role == "" {
		return false
	}
	return auth.CanString(role, action)
}

func NewCollectionHandler(database *db.DB) *CollectionHandler {
	return &CollectionHandler{db: database}
}

// GET /api/collections?workspaceId=xxx
func (h *CollectionHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspaceId")
	cols, err := h.db.ListCollections(wsID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, cols)
}

// POST /api/collections
func (h *CollectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if req.Color == "" {
		req.Color = "#4ade80"
	}
	wsID := r.URL.Query().Get("workspaceId")
	if !h.canDo(r, wsID, auth.ActionCollectionCreate) {
		jsonError(w, "insufficient permissions", http.StatusForbidden)
		return
	}
	// Enforce collection quota (uses workspace owner's plan)
	if wsID != "" {
		if claims := getClaimsFromRequest(r); claims != nil {
			ws, _ := h.db.GetWorkspace(wsID)
			ownerID := claims.UserID
			if ws != nil {
				ownerID = ws.OwnerID
			}
			if qErr := h.db.CheckCollectionQuota(wsID, ownerID); qErr != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": qErr.Error(), "quota": qErr, "upgradeRequired": true})
				return
			}
		}
	}
	col, err := h.db.CreateCollection(generateID(), wsID, req.Name, req.Description, req.Color)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, col)
}

// PUT /api/collections/{id}
func (h *CollectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if col, _ := h.db.GetCollection(id); col != nil {
		if !h.canDo(r, col.WorkspaceID, auth.ActionCollectionUpdate) {
			jsonError(w, "insufficient permissions", http.StatusForbidden)
			return
		}
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := h.db.UpdateCollection(id, req.Name, req.Description, req.Color); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	col, _ := h.db.GetCollection(id)
	jsonOK(w, col)
}

// DELETE /api/collections/{id}
func (h *CollectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if col, _ := h.db.GetCollection(id); col != nil {
		if !h.canDo(r, col.WorkspaceID, auth.ActionCollectionDelete) {
			jsonError(w, "insufficient permissions", http.StatusForbidden)
			return
		}
	}
	if err := h.db.DeleteCollection(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"deleted": id})
}

// POST /api/collections/{id}/requests
func (h *CollectionHandler) SaveRequest(w http.ResponseWriter, r *http.Request) {
	collectionID := mux.Vars(r)["id"]

	// Permission: derive workspace from collection
	wsID := h.db.WorkspaceIDForCollection(collectionID)
	if !h.canDo(r, wsID, auth.ActionRequestCreate) {
		jsonError(w, "insufficient permissions", http.StatusForbidden)
		return
	}

	var req db.SavedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.CollectionID = collectionID
	if req.ID == "" {
		req.ID = generateID()
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}

	saved, err := h.db.SaveRequest(&req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, saved)
}

// PUT /api/collections/requests/{reqId}
func (h *CollectionHandler) UpdateRequest(w http.ResponseWriter, r *http.Request) {
	reqID := mux.Vars(r)["reqId"]

	// Permission: derive workspace from existing request's collection
	existing, _ := h.db.GetSavedRequest(reqID)
	if existing != nil {
		wsID := h.db.WorkspaceIDForCollection(existing.CollectionID)
		if !h.canDo(r, wsID, auth.ActionRequestUpdate) {
			jsonError(w, "insufficient permissions", http.StatusForbidden)
			return
		}
	}

	var req db.SavedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.ID = reqID
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}

	saved, err := h.db.SaveRequest(&req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, saved)
}

// DELETE /api/collections/requests/{reqId}
func (h *CollectionHandler) DeleteRequest(w http.ResponseWriter, r *http.Request) {
	reqID := mux.Vars(r)["reqId"]

	// Permission: derive workspace from existing request's collection
	existing, _ := h.db.GetSavedRequest(reqID)
	if existing != nil {
		wsID := h.db.WorkspaceIDForCollection(existing.CollectionID)
		if !h.canDo(r, wsID, auth.ActionRequestDelete) {
			jsonError(w, "insufficient permissions", http.StatusForbidden)
			return
		}
	}

	if err := h.db.DeleteSavedRequest(reqID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"deleted": reqID})
}

// POST /api/collections/requests/{reqId}/move
func (h *CollectionHandler) MoveRequest(w http.ResponseWriter, r *http.Request) {
	reqID := mux.Vars(r)["reqId"]
	var body struct {
		CollectionID string `json:"collectionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CollectionID == "" {
		jsonError(w, "collectionId required", http.StatusBadRequest)
		return
	}

	// Permission: user must have update rights in BOTH source and target workspace
	existing, _ := h.db.GetSavedRequest(reqID)
	if existing != nil {
		srcWsID := h.db.WorkspaceIDForCollection(existing.CollectionID)
		dstWsID := h.db.WorkspaceIDForCollection(body.CollectionID)
		if !h.canDo(r, srcWsID, auth.ActionRequestUpdate) || !h.canDo(r, dstWsID, auth.ActionRequestCreate) {
			jsonError(w, "insufficient permissions", http.StatusForbidden)
			return
		}
	}

	if err := h.db.MoveRequest(reqID, body.CollectionID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"moved": reqID})
}

// GET /api/collections/export  — export all as JSON
func (h *CollectionHandler) Export(w http.ResponseWriter, r *http.Request) {
	if claims := getClaimsFromRequest(r); claims != nil {
		if fErr := h.db.CheckFeatureAccess(claims.UserID, "export_import"); fErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": fErr.Error(), "upgradeRequired": true})
			return
		}
	}
	wsID := r.URL.Query().Get("workspaceId")
	cols, err := h.db.ListCollections(wsID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="grpc-inspector-collections.json"`)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version":     1,
		"collections": cols,
	})
}

// POST /api/collections/import  — import from JSON
func (h *CollectionHandler) Import(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Collections []*db.Collection `json:"collections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	imported := 0
	for _, col := range body.Collections {
		wsID := r.URL.Query().Get("workspaceId")
		newCol, err := h.db.CreateCollection(generateID(), wsID, col.Name, col.Description, col.Color)
		if err != nil {
			continue
		}
		for _, req := range col.Requests {
			req.ID = generateID()
			req.CollectionID = newCol.ID
			h.db.SaveRequest(req)
			imported++
		}
	}
	jsonOK(w, map[string]int{"imported": imported, "collections": len(body.Collections)})
}

// POST /api/collections/migrate-orphans?workspaceId=xxx
// Assigns all workspace_id=NULL collections to the given workspace.
func (h *CollectionHandler) MigrateOrphans(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("workspaceId")
	if wsID == "" {
		jsonError(w, "workspaceId required", http.StatusBadRequest)
		return
	}
	n, err := h.db.MigrateOrphanCollections(wsID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"migrated": n})
}
