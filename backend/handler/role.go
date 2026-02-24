package handler

import (
	"net/http"

	"grpc-inspector/db"
)

type RoleHandler struct {
	db *db.DB
}

func NewRoleHandler(database *db.DB) *RoleHandler {
	return &RoleHandler{db: database}
}

// GET /api/roles — returns all roles with their permissions
func (h *RoleHandler) List(w http.ResponseWriter, r *http.Request) {
	roles, err := h.db.ListRoles()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, roles)
}
