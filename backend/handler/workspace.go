package handler

import (
	"log"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"grpc-inspector/auth"
	"grpc-inspector/db"
	"grpc-inspector/email"
)

type WorkspaceHandler struct {
	db       *db.DB
	emailCfg *email.Config
}

func (h *WorkspaceHandler) requirePermission(w http.ResponseWriter, workspaceID, userID string, action auth.Action) bool {
	role, err := h.db.GetMemberRole(workspaceID, userID)
	if err != nil || role == "" {
		jsonError(w, "not a member of this workspace", http.StatusForbidden)
		return false
	}
	if !auth.CanString(role, action) {
		jsonError(w, "insufficient permissions: "+string(action), http.StatusForbidden)
		return false
	}
	return true
}

func NewWorkspaceHandler(database *db.DB) *WorkspaceHandler {
	return &WorkspaceHandler{db: database, emailCfg: email.LoadConfig()}
}

func (h *WorkspaceHandler) currentUserID(r *http.Request) string {
	if c := auth.GetClaims(r.Context()); c != nil {
		return c.UserID
	}
	return ""
}

// GET /api/workspaces
func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := h.currentUserID(r)
	workspaces, err := h.db.ListWorkspacesForUser(uid)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, workspaces)
}

// POST /api/workspaces
func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := h.currentUserID(r)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if qErr := h.db.CheckWorkspaceQuota(uid); qErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": qErr.Error(), "quota": qErr, "upgradeRequired": true})
		return
	}
	ws, err := h.db.CreateWorkspace(generateID(), req.Name, req.Description, uid)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, ws)
}

// GET /api/workspaces/{id}
func (h *WorkspaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	uid := h.currentUserID(r)

	role, _ := h.db.GetMemberRole(id, uid)
	if role == "" {
		jsonError(w, "not a member", http.StatusForbidden)
		return
	}
	ws, err := h.db.GetWorkspace(id)
	if err != nil || ws == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, ws)
}

// PUT /api/workspaces/{id}
func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	uid := h.currentUserID(r)

	if !h.requirePermission(w, id, uid, auth.ActionWorkspaceUpdate) {
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	h.db.UpdateWorkspace(id, req.Name, req.Description)
	ws, _ := h.db.GetWorkspace(id)
	jsonOK(w, ws)
}

// DELETE /api/workspaces/{id}
func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	uid := h.currentUserID(r)

	if !h.requirePermission(w, id, uid, auth.ActionWorkspaceDelete) {
		return
	}
	h.db.DeleteWorkspace(id)
	jsonOK(w, map[string]string{"deleted": id})
}

// ── Members ───────────────────────────────────────────────────────────────────

// GET /api/workspaces/{id}/members
func (h *WorkspaceHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	uid := h.currentUserID(r)
	if !h.requirePermission(w, id, uid, auth.ActionCollectionRead) {
		return
	}
	members, err := h.db.ListWorkspaceMembers(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if members == nil {
		members = []*db.WorkspaceMember{}
	}
	jsonOK(w, members)
}

// PUT /api/workspaces/{id}/members/{userId}
func (h *WorkspaceHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wsID, targetUID := vars["id"], vars["userId"]
	uid := h.currentUserID(r)

	if !h.requirePermission(w, wsID, uid, auth.ActionMemberSetRole) {
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if !auth.ValidRole(req.Role) || req.Role == "owner" {
		jsonError(w, "invalid role", http.StatusBadRequest)
		return
	}

	// Can't change owner's role
	ws, _ := h.db.GetWorkspace(wsID)
	if ws != nil && targetUID == ws.OwnerID {
		jsonError(w, "cannot change owner role", http.StatusBadRequest)
		return
	}

	h.db.UpdateMemberRole(wsID, targetUID, req.Role)
	jsonOK(w, map[string]string{"updated": targetUID, "role": req.Role})
}

// DELETE /api/workspaces/{id}/members/{userId}
func (h *WorkspaceHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wsID, targetUID := vars["id"], vars["userId"]
	uid := h.currentUserID(r)

	// Allow self-remove (leave workspace) or members with remove permission
	if uid != targetUID {
		if !h.requirePermission(w, wsID, uid, auth.ActionMemberRemove) {
			return
		}
	}

	ws, _ := h.db.GetWorkspace(wsID)
	if ws != nil && targetUID == ws.OwnerID {
		jsonError(w, "cannot remove workspace owner", http.StatusBadRequest)
		return
	}

	h.db.RemoveMember(wsID, targetUID)
	jsonOK(w, map[string]string{"removed": targetUID})
}

// ── Invites ───────────────────────────────────────────────────────────────────

// POST /api/workspaces/{id}/invites
func (h *WorkspaceHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	wsID := mux.Vars(r)["id"]
	uid := h.currentUserID(r)

	if !h.requirePermission(w, wsID, uid, auth.ActionMemberInvite) {
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Email == "" {
		jsonError(w, "email required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "editor"
	}

	// Check member quota
	if qErr := h.db.CheckMemberQuota(wsID); qErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": qErr.Error(), "quota": qErr, "upgradeRequired": true})
		return
	}

	token := auth.GenerateToken32()
	invite, err := h.db.CreateInvite(
		generateID(), wsID, req.Email, req.Role, token, uid,
		time.Now().Add(7*24*time.Hour),
	)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	inviteURL := "/invite/" + token

	// Respond immediately — fire email in background goroutine
	emailWillSend := h.emailCfg != nil

	if h.emailCfg != nil {
		// Capture values for goroutine
		emailCfg := h.emailCfg
		recipient := req.Email
		role := req.Role
		fullInviteURL := h.emailCfg.AppURL + inviteURL

		go func() {
			ws, _ := h.db.GetWorkspace(wsID)
			inviter, _ := h.db.GetUserByID(uid)
			wsName := ""
			inviterName := ""
			if ws != nil { wsName = ws.Name }
			if inviter != nil { inviterName = inviter.Name }

			if err := emailCfg.SendInvite(recipient, email.InviteData{
				InvitedBy:     inviterName,
				WorkspaceName: wsName,
				Role:          role,
				InviteURL:     fullInviteURL,
				AppURL:        emailCfg.AppURL,
			}); err != nil {
				log.Printf("⚠️  invite email to %s failed: %v", recipient, err)
			} else {
				log.Printf("✉️  invite email sent to %s", recipient)
			}
		}()
	}

	jsonOK(w, map[string]interface{}{
		"invite":    invite,
		"inviteUrl": inviteURL,
		"emailSent": emailWillSend, // true = email is being sent async
	})
}

// GET /api/workspaces/{id}/invites
func (h *WorkspaceHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
	wsID := mux.Vars(r)["id"]
	uid := h.currentUserID(r)

	if !h.requirePermission(w, wsID, uid, auth.ActionMemberInvite) {
		return
	}

	invites, _ := h.db.ListInvites(wsID)
	if invites == nil {
		invites = []*db.WorkspaceInvite{}
	}
	jsonOK(w, invites)
}

// POST /api/invites/{token}/accept
func (h *WorkspaceHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := mux.Vars(r)["token"]

	// AcceptInvite is a public endpoint (no middleware) so we manually parse
	// the JWT if present in the Authorization header
	uid := h.currentUserID(r)
	if uid == "" {
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				if claims, err := auth.ParseToken(parts[1]); err == nil {
					uid = claims.UserID
				}
			}
		}
	}

	invite, err := h.db.GetInviteByToken(token)
	if err != nil || invite == nil {
		jsonError(w, "invite not found", http.StatusNotFound)
		return
	}
	if invite.Used {
		jsonError(w, "invite already used", http.StatusBadRequest)
		return
	}
	if time.Now().After(invite.ExpiresAt) {
		jsonError(w, "invite expired", http.StatusBadRequest)
		return
	}

	// If not logged in, require registration first
	if uid == "" {
		// Return invite info so frontend can show register form
		jsonOK(w, map[string]interface{}{
			"requiresAuth": true,
			"invite":       invite,
		})
		return
	}

	if err := h.db.AddMember(invite.WorkspaceID, uid, invite.Role); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.db.MarkInviteUsed(token)

	ws, _ := h.db.GetWorkspace(invite.WorkspaceID)
	if ws != nil {
		members, _ := h.db.ListWorkspaceMembers(ws.ID)
		ws.Members = members
	}
	jsonOK(w, map[string]interface{}{
		"workspace": ws,
		"role":      invite.Role,
	})
}
