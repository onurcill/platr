package db

import "time"

type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	Description string    `json:"description"`
	Rank        int       `json:"rank"`
	IsSystem    bool      `json:"isSystem"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"createdAt"`
}

type RolePermission struct {
	ID        string    `json:"id"`
	RoleID    string    `json:"roleId"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"createdAt"`
}

func (d *DB) ListRoles() ([]*Role, error) {
	rows, err := d.Query(`
		SELECT id, name, display_name, description, rank, is_system, created_at
		FROM roles ORDER BY rank DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		r := &Role{}
		var isSystem int
		if err := rows.Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &r.Rank, &isSystem, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.IsSystem = isSystem == 1
		roles = append(roles, r)
	}

	// Load permissions for each role
	for _, r := range roles {
		perms, err := d.ListRolePermissions(r.ID)
		if err != nil {
			return nil, err
		}
		r.Permissions = perms
	}

	if roles == nil {
		roles = []*Role{}
	}
	return roles, nil
}

func (d *DB) GetRoleByName(name string) (*Role, error) {
	r := &Role{}
	var isSystem int
	err := d.QueryRow(`
		SELECT id, name, display_name, description, rank, is_system, created_at
		FROM roles WHERE name=?`, name,
	).Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &r.Rank, &isSystem, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem == 1
	perms, _ := d.ListRolePermissions(r.ID)
	r.Permissions = perms
	return r, nil
}

func (d *DB) ListRolePermissions(roleID string) ([]string, error) {
	rows, err := d.Query(`SELECT action FROM role_permissions WHERE role_id=? ORDER BY action`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, nil
}

// CanByDB checks permission using DB roles table — single source of truth.
func (d *DB) CanByDB(roleName string, action string) (bool, error) {
	var count int
	err := d.QueryRow(`
		SELECT COUNT(*) FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id
		WHERE r.name=? AND rp.action=?`, roleName, action,
	).Scan(&count)
	return count > 0, err
}
