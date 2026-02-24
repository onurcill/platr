package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"grpc-inspector/auth"
	"grpc-inspector/db"
)

type AuthHandler struct {
	db *db.DB
}


func enrichWorkspaces(database *db.DB, workspaces []*db.Workspace) []*db.Workspace {
	for _, ws := range workspaces {
		members, err := database.ListWorkspaceMembers(ws.ID)
		if err == nil {
			ws.Members = members
		}
	}
	return workspaces
}

func NewAuthHandler(database *db.DB) *AuthHandler {
	return &AuthHandler{db: database}
}

// POST /api/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if req.Email == "" || req.Password == "" || req.Name == "" {
		jsonError(w, "email, name and password required", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 8 {
		jsonError(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Check existing
	existing, _ := h.db.GetUserByEmail(req.Email)
	if existing != nil {
		jsonError(w, "email already registered", http.StatusConflict)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	user, err := h.db.CreateUser(generateID(), req.Email, req.Name, hash)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create personal workspace
	wsID := generateID()
	ws, err := h.db.CreateWorkspace(wsID, req.Name+"'s Workspace", "Personal workspace", user.ID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// ws already has members from CreateWorkspace -> GetWorkspace
	token, err := auth.GenerateToken(user.ID, user.Email, user.Name)
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]interface{}{
		"token":     token,
		"user":      user,
		"workspace": ws,
	})
}

// POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	user, err := h.db.GetUserByEmail(req.Email)
	if err != nil || user == nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(user.ID, user.Email, user.Name)
	if err != nil {
		jsonError(w, "token generation failed", http.StatusInternalServerError)
		return
	}

	workspaces, _ := h.db.ListWorkspacesForUser(user.ID)
	enrichWorkspaces(h.db, workspaces)

	jsonOK(w, map[string]interface{}{
		"token":      token,
		"user":       user,
		"workspaces": workspaces,
	})
}

// GET /api/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.db.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	workspaces, _ := h.db.ListWorkspacesForUser(user.ID)
	enrichWorkspaces(h.db, workspaces)

	jsonOK(w, map[string]interface{}{
		"user":       user,
		"workspaces": workspaces,
	})
}
