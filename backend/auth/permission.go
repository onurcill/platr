package auth

// Role hierarchy (higher index = more permissions)
type Role string

const (
	RoleGuest  Role = "guest"
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleAdmin  Role = "admin"
	RoleOwner  Role = "owner"
)

// Action represents a permission-gated operation
type Action string

const (
	// Workspace
	ActionWorkspaceUpdate Action = "workspace:update"
	ActionWorkspaceDelete Action = "workspace:delete"

	// Members
	ActionMemberInvite  Action = "member:invite"
	ActionMemberRemove  Action = "member:remove"
	ActionMemberSetRole Action = "member:set_role"

	// Collections
	ActionCollectionCreate Action = "collection:create"
	ActionCollectionUpdate Action = "collection:update"
	ActionCollectionDelete Action = "collection:delete"
	ActionCollectionRead   Action = "collection:read"

	// Requests (saved)
	ActionRequestCreate Action = "request:create"
	ActionRequestUpdate Action = "request:update"
	ActionRequestDelete Action = "request:delete"

	// Invoke
	ActionInvoke Action = "invoke"

	// Environment
	ActionEnvironmentManage Action = "environment:manage"
	ActionEnvironmentRead   Action = "environment:read"

	// Proto
	ActionProtoUpload Action = "proto:upload"

	// History
	ActionHistoryRead Action = "history:read"
)

// permissionMatrix defines which roles can perform which actions.
// Uses minimum role required — any role >= that level is allowed.
var permissionMatrix = map[Action]Role{
	// Workspace management
	ActionWorkspaceUpdate: RoleOwner,
	ActionWorkspaceDelete: RoleOwner,

	// Member management
	ActionMemberInvite:  RoleAdmin,
	ActionMemberRemove:  RoleAdmin,
	ActionMemberSetRole: RoleAdmin,

	// Collections — editors and above
	ActionCollectionCreate: RoleEditor,
	ActionCollectionUpdate: RoleEditor,
	ActionCollectionDelete: RoleEditor,
	ActionCollectionRead:   RoleGuest,

	// Saved requests
	ActionRequestCreate: RoleEditor,
	ActionRequestUpdate: RoleEditor,
	ActionRequestDelete: RoleEditor,

	// Invoke — viewers and above (guests can't invoke)
	ActionInvoke: RoleViewer,

	// Environment
	ActionEnvironmentManage: RoleEditor,
	ActionEnvironmentRead:   RoleViewer,

	// Proto upload
	ActionProtoUpload: RoleEditor,

	// History
	ActionHistoryRead: RoleViewer,
}

// roleRank maps roles to numeric rank for comparison
var roleRank = map[Role]int{
	RoleGuest:  0,
	RoleViewer: 1,
	RoleEditor: 2,
	RoleAdmin:  3,
	RoleOwner:  4,
}

// Can returns true if the given role is allowed to perform the action.
func Can(role Role, action Action) bool {
	required, ok := permissionMatrix[action]
	if !ok {
		return false // unknown action — deny
	}
	return roleRank[role] >= roleRank[required]
}

// CanString is a convenience wrapper for string roles from DB.
func CanString(roleStr string, action Action) bool {
	return Can(Role(roleStr), action)
}

// ValidRole returns true if the role string is a known role.
func ValidRole(r string) bool {
	switch Role(r) {
	case RoleGuest, RoleViewer, RoleEditor, RoleAdmin, RoleOwner:
		return true
	}
	return false
}

// AssignableRoles returns roles that can be assigned (not owner — owner is set at creation).
var AssignableRoles = []string{
	string(RoleGuest),
	string(RoleViewer),
	string(RoleEditor),
	string(RoleAdmin),
}

// RoleDescription returns a human-readable description for each role.
var RoleDescription = map[string]string{
	string(RoleOwner):  "Full control. Can delete workspace and transfer ownership.",
	string(RoleAdmin):  "Can manage members and invites. Cannot delete workspace.",
	string(RoleEditor): "Can create and edit collections, upload protos, manage environments.",
	string(RoleViewer): "Can send requests and view history. Cannot edit collections.",
	string(RoleGuest):  "Read-only access to collections. Cannot send requests.",
}
