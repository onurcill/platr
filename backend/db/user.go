package db

import (
	"database/sql"
	"time"
)

// User — no global role field; roles live in workspace_members.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (d *DB) CreateUser(id, email, name, passwordHash string) (*User, error) {
	_, err := d.Exec(
		`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, ?)`,
		id, email, name, passwordHash,
	)
	if err != nil {
		return nil, err
	}
	return d.GetUserByID(id)
}

func (d *DB) GetUserByID(id string) (*User, error) {
	u := &User{}
	err := d.QueryRow(
		`SELECT id, email, name, password_hash, created_at FROM users WHERE id=?`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (d *DB) GetUserByEmail(email string) (*User, error) {
	u := &User{}
	err := d.QueryRow(
		`SELECT id, email, name, password_hash, created_at FROM users WHERE email=?`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (d *DB) ListUsers() ([]*User, error) {
	rows, err := d.Query(`SELECT id, email, name, created_at FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *DB) UserCount() (int, error) {
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// ── Workspace ─────────────────────────────────────────────────────────────────

type Workspace struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	OwnerID     string             `json:"ownerId"`
	Members     []*WorkspaceMember `json:"members,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
}

type WorkspaceMember struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	UserID      string    `json:"userId"`
	UserEmail   string    `json:"userEmail"`
	UserName    string    `json:"userName"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joinedAt"`
}

func (d *DB) CreateWorkspace(id, name, description, ownerID string) (*Workspace, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO workspaces (id, name, description, owner_id) VALUES (?, ?, ?, ?)`,
		id, name, description, ownerID,
	); err != nil {
		return nil, err
	}

	// Owner is automatically a member — use random PK, not deterministic
	if _, err := tx.Exec(
		`INSERT INTO workspace_members (id, workspace_id, user_id, role)
		 VALUES (lower(hex(randomblob(8))), ?, ?, 'owner')`,
		id, ownerID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetWorkspace(id)
}

func (d *DB) GetWorkspace(id string) (*Workspace, error) {
	w := &Workspace{}
	err := d.QueryRow(
		`SELECT id, name, description, owner_id, created_at FROM workspaces WHERE id=?`, id,
	).Scan(&w.ID, &w.Name, &w.Description, &w.OwnerID, &w.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	members, err := d.ListWorkspaceMembers(id)
	if err != nil {
		return nil, err
	}
	w.Members = members
	return w, nil
}

func (d *DB) ListWorkspacesForUser(userID string) ([]*Workspace, error) {
	rows, err := d.Query(`
		SELECT w.id, w.name, w.description, w.owner_id, w.created_at
		FROM workspaces w
		JOIN workspace_members wm ON wm.workspace_id = w.id
		WHERE wm.user_id = ?
		ORDER BY w.created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []*Workspace
	for rows.Next() {
		w := &Workspace{}
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &w.OwnerID, &w.CreatedAt); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, w)
	}
	if workspaces == nil {
		workspaces = []*Workspace{}
	}
	return workspaces, nil
}

func (d *DB) UpdateWorkspace(id, name, description string) error {
	_, err := d.Exec(`UPDATE workspaces SET name=?, description=? WHERE id=?`, name, description, id)
	return err
}

func (d *DB) DeleteWorkspace(id string) error {
	_, err := d.Exec(`DELETE FROM workspaces WHERE id=?`, id)
	return err
}

// ── Members ───────────────────────────────────────────────────────────────────

func (d *DB) ListWorkspaceMembers(workspaceID string) ([]*WorkspaceMember, error) {
	rows, err := d.Query(`
		SELECT wm.id, wm.workspace_id, wm.user_id, u.email, u.name, wm.role, wm.joined_at
		FROM workspace_members wm
		JOIN users u ON u.id = wm.user_id
		WHERE wm.workspace_id = ?
		ORDER BY wm.joined_at ASC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*WorkspaceMember
	for rows.Next() {
		m := &WorkspaceMember{}
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.UserEmail, &m.UserName, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

func (d *DB) GetMemberRole(workspaceID, userID string) (string, error) {
	var role string
	err := d.QueryRow(
		`SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?`,
		workspaceID, userID,
	).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return role, err
}

func (d *DB) AddMember(workspaceID, userID, role string) error {
	_, err := d.Exec(
		`INSERT OR REPLACE INTO workspace_members (id, workspace_id, user_id, role)
		 VALUES (lower(hex(randomblob(8))), ?, ?, ?)`,
		workspaceID, userID, role,
	)
	return err
}

func (d *DB) UpdateMemberRole(workspaceID, userID, role string) error {
	_, err := d.Exec(
		`UPDATE workspace_members SET role=? WHERE workspace_id=? AND user_id=?`,
		role, workspaceID, userID,
	)
	return err
}

func (d *DB) RemoveMember(workspaceID, userID string) error {
	_, err := d.Exec(
		`DELETE FROM workspace_members WHERE workspace_id=? AND user_id=?`,
		workspaceID, userID,
	)
	return err
}

// ── Invites ───────────────────────────────────────────────────────────────────

type WorkspaceInvite struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Token       string    `json:"token"`
	CreatedBy   string    `json:"createdBy"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Used        bool      `json:"used"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (d *DB) CreateInvite(id, workspaceID, email, role, token, createdBy string, expiresAt time.Time) (*WorkspaceInvite, error) {
	_, err := d.Exec(
		`INSERT INTO workspace_invites (id, workspace_id, email, role, token, created_by, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, workspaceID, email, role, token, createdBy, expiresAt,
	)
	if err != nil {
		return nil, err
	}
	return &WorkspaceInvite{
		ID: id, WorkspaceID: workspaceID, Email: email,
		Role: role, Token: token, CreatedBy: createdBy, ExpiresAt: expiresAt,
	}, nil
}

func (d *DB) GetInviteByToken(token string) (*WorkspaceInvite, error) {
	inv := &WorkspaceInvite{}
	var used int
	err := d.QueryRow(
		`SELECT id, workspace_id, email, role, token, created_by, expires_at, used, created_at
		 FROM workspace_invites WHERE token=?`, token,
	).Scan(&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.Token,
		&inv.CreatedBy, &inv.ExpiresAt, &used, &inv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	inv.Used = used == 1
	return inv, err
}

func (d *DB) MarkInviteUsed(token string) error {
	_, err := d.Exec(`UPDATE workspace_invites SET used=1 WHERE token=?`, token)
	return err
}

func (d *DB) ListInvites(workspaceID string) ([]*WorkspaceInvite, error) {
	rows, err := d.Query(
		`SELECT id, workspace_id, email, role, token, created_by, expires_at, used, created_at
		 FROM workspace_invites WHERE workspace_id=? ORDER BY created_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invites []*WorkspaceInvite
	for rows.Next() {
		inv := &WorkspaceInvite{}
		var used int
		if err := rows.Scan(&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.Token,
			&inv.CreatedBy, &inv.ExpiresAt, &used, &inv.CreatedAt); err != nil {
			return nil, err
		}
		inv.Used = used == 1
		invites = append(invites, inv)
	}
	return invites, nil
}

