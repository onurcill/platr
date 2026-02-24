package db

import (
	"database/sql"
	"time"
)

type Environment struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspaceId"`
	Name        string     `json:"name"`
	Color       string     `json:"color"`
	Variables   []*EnvVar  `json:"variables"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type EnvVar struct {
	ID     string `json:"id"`
	EnvID  string `json:"envId"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

func (d *DB) CreateEnvironment(id, workspaceID, name, color string) (*Environment, error) {
	_, err := d.Exec(
		`INSERT INTO environments (id, workspace_id, name, color) VALUES (?, ?, ?, ?)`,
		id, workspaceID, name, color,
	)
	if err != nil {
		return nil, err
	}
	return d.GetEnvironment(id)
}

func (d *DB) GetEnvironment(id string) (*Environment, error) {
	env := &Environment{}
	err := d.QueryRow(
		`SELECT id, workspace_id, name, color, created_at, updated_at FROM environments WHERE id=?`, id,
	).Scan(&env.ID, &env.WorkspaceID, &env.Name, &env.Color, &env.CreatedAt, &env.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	vars, err := d.ListEnvVars(id)
	if err != nil {
		return nil, err
	}
	env.Variables = vars
	if env.Variables == nil {
		env.Variables = []*EnvVar{}
	}
	return env, nil
}

// ListEnvironments filters by workspace — never returns cross-workspace envs.
func (d *DB) ListEnvironments(workspaceID string) ([]*Environment, error) {
	rows, err := d.Query(
		`SELECT id, workspace_id, name, color, created_at, updated_at
		 FROM environments WHERE workspace_id=? ORDER BY created_at ASC`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []*Environment
	for rows.Next() {
		env := &Environment{}
		if err := rows.Scan(&env.ID, &env.WorkspaceID, &env.Name, &env.Color, &env.CreatedAt, &env.UpdatedAt); err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Batch load variables with a single JOIN query
	if len(envs) > 0 {
		envMap := map[string]*Environment{}
		for _, e := range envs {
			e.Variables = []*EnvVar{}
			envMap[e.ID] = e
		}
		varRows, err := d.Query(`
			SELECT ev.id, ev.env_id, ev.key, ev.value, ev.secret
			FROM env_variables ev
			INNER JOIN environments e ON e.id = ev.env_id
			WHERE e.workspace_id = ?
			ORDER BY ev.rowid ASC`, workspaceID)
		if err != nil {
			return nil, err
		}
		defer varRows.Close()
		for varRows.Next() {
			v := &EnvVar{}
			var secret int
			if err := varRows.Scan(&v.ID, &v.EnvID, &v.Key, &v.Value, &secret); err != nil {
				return nil, err
			}
			v.Secret = secret == 1
			if e, ok := envMap[v.EnvID]; ok {
				e.Variables = append(e.Variables, v)
			}
		}
	}

	if envs == nil {
		envs = []*Environment{}
	}
	return envs, nil
}

func (d *DB) UpdateEnvironment(id, name, color string) error {
	_, err := d.Exec(
		`UPDATE environments SET name=?, color=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		name, color, id,
	)
	return err
}

func (d *DB) DeleteEnvironment(id string) error {
	_, err := d.Exec(`DELETE FROM environments WHERE id=?`, id)
	return err
}

func (d *DB) ListEnvVars(envID string) ([]*EnvVar, error) {
	rows, err := d.Query(
		`SELECT id, env_id, key, value, secret FROM env_variables WHERE env_id=? ORDER BY rowid ASC`,
		envID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vars []*EnvVar
	for rows.Next() {
		v := &EnvVar{}
		var secret int
		if err := rows.Scan(&v.ID, &v.EnvID, &v.Key, &v.Value, &secret); err != nil {
			return nil, err
		}
		v.Secret = secret == 1
		vars = append(vars, v)
	}
	return vars, nil
}

func (d *DB) SetEnvVars(envID string, vars []*EnvVar) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM env_variables WHERE env_id=?`, envID); err != nil {
		return err
	}
	for _, v := range vars {
		secret := 0
		if v.Secret {
			secret = 1
		}
		// Always generate fresh ID to avoid PK conflicts on re-save
		if _, err := tx.Exec(
			`INSERT INTO env_variables (id, env_id, key, value, secret) VALUES (lower(hex(randomblob(8))), ?, ?, ?, ?)`,
			envID, v.Key, v.Value, secret,
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		`UPDATE environments SET updated_at=CURRENT_TIMESTAMP WHERE id=?`, envID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func ResolveVariables(s string, vars []*EnvVar) string {
	for _, v := range vars {
		placeholder := "{{" + v.Key + "}}"
		result := ""
		remaining := s
		for {
			idx := indexOf(remaining, placeholder)
			if idx < 0 {
				result += remaining
				break
			}
			result += remaining[:idx] + v.Value
			remaining = remaining[idx+len(placeholder):]
		}
		s = result
	}
	return s
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
