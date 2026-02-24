package handler

import (
	"net/http"

	"github.com/gorilla/mux"

	"grpc-inspector/auth"
	"grpc-inspector/db"
)

type HistoryHandler struct {
	db *db.DB
}

func NewHistoryHandler(database *db.DB) *HistoryHandler {
	return &HistoryHandler{db: database}
}

func (h *HistoryHandler) workspaceID(r *http.Request) string {
	return r.URL.Query().Get("workspaceId")
}

// GET /api/history?workspaceId=xxx
func (h *HistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := h.workspaceID(r)
	if wsID == "" {
		jsonOK(w, []*db.HistoryEntry{})
		return
	}
	// Membership check — prevent workspace ID enumeration
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	role, _ := h.db.GetMemberRole(wsID, claims.UserID)
	if !auth.CanString(role, auth.ActionHistoryRead) {
		jsonError(w, "insufficient permissions", http.StatusForbidden)
		return
	}
	entries, err := h.db.ListHistory(wsID, 200)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, entries)
}

// DELETE /api/history/:id?workspaceId=xxx
func (h *HistoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := h.workspaceID(r)
	id := mux.Vars(r)["id"]
	if err := h.db.DeleteHistoryEntry(id, wsID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/history?workspaceId=xxx
func (h *HistoryHandler) Clear(w http.ResponseWriter, r *http.Request) {
	wsID := h.workspaceID(r)
	if wsID == "" {
		jsonError(w, "workspaceId required", http.StatusBadRequest)
		return
	}
	if err := h.db.ClearHistory(wsID); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PersistHistory is called internally by invoke handlers.
// It saves the entry and then prunes old entries per the owner's retention policy.
func PersistHistory(database *db.DB, r *http.Request, e *db.HistoryEntry) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || e.WorkspaceID == "" {
		return
	}
	e.ID = generateID()
	e.UserID = claims.UserID
	_ = database.AddHistoryEntry(e)

	// Prune expired history asynchronously — don't block the response
	wsID := e.WorkspaceID
	ownerID := claims.UserID
	go func() {
		ws, err := database.GetWorkspace(wsID)
		if err == nil && ws != nil {
			ownerID = ws.OwnerID
		}
		database.PruneHistoryByRetention(wsID, ownerID)
	}()
}
