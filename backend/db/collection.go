package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Collection struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Color       string          `json:"color"`
	SortOrder   int             `json:"sortOrder"`
	Requests    []*SavedRequest `json:"requests"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type SavedRequest struct {
	ID           string            `json:"id"`
	CollectionID string            `json:"collectionId"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Service      string            `json:"service"`
	Method       string            `json:"method"`
	Body         string            `json:"body"`
	Metadata     map[string]string `json:"metadata"`
	ConnAddress  string            `json:"connAddress"`
	EnvID        *string           `json:"envId"`
	SortOrder    int               `json:"sortOrder"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// ListCollections uses a single JOIN query to avoid N+1.
func (d *DB) ListCollections(workspaceID string) ([]*Collection, error) {
	// Fetch collections
	rows, err := d.Query(`
		SELECT id, workspace_id, name, description, color, sort_order, created_at, updated_at
		FROM collections
		WHERE workspace_id = ?
		ORDER BY sort_order ASC, created_at ASC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colMap := map[string]*Collection{}
	var colOrder []string
	for rows.Next() {
		c := &Collection{Requests: []*SavedRequest{}}
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Description, &c.Color, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		colMap[c.ID] = c
		colOrder = append(colOrder, c.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(colOrder) == 0 {
		return []*Collection{}, nil
	}

	// Fetch all requests for these collections in one query via JOIN
	reqRows, err := d.Query(`
		SELECT sr.id, sr.collection_id, sr.name, sr.description,
		       sr.service, sr.method, sr.body, sr.metadata,
		       sr.conn_address, sr.env_id, sr.sort_order, sr.created_at, sr.updated_at
		FROM saved_requests sr
		INNER JOIN collections c ON c.id = sr.collection_id
		WHERE c.workspace_id = ?
		ORDER BY sr.sort_order ASC, sr.created_at ASC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer reqRows.Close()

	for reqRows.Next() {
		r := &SavedRequest{}
		var metaJSON string
		var envID sql.NullString
		if err := reqRows.Scan(
			&r.ID, &r.CollectionID, &r.Name, &r.Description,
			&r.Service, &r.Method, &r.Body, &metaJSON,
			&r.ConnAddress, &envID, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if envID.Valid {
			r.EnvID = &envID.String
		}
		if err := json.Unmarshal([]byte(metaJSON), &r.Metadata); err != nil {
			r.Metadata = map[string]string{}
		}
		if col, ok := colMap[r.CollectionID]; ok {
			col.Requests = append(col.Requests, r)
		}
	}

	cols := make([]*Collection, 0, len(colOrder))
	for _, id := range colOrder {
		cols = append(cols, colMap[id])
	}
	return cols, nil
}

func (d *DB) CreateCollection(id, workspaceID, name, description, color string) (*Collection, error) {
	_, err := d.Exec(
		`INSERT INTO collections (id, workspace_id, name, description, color) VALUES (?, ?, ?, ?, ?)`,
		id, workspaceID, name, description, color,
	)
	if err != nil {
		return nil, err
	}
	return d.GetCollection(id)
}

func (d *DB) GetCollection(id string) (*Collection, error) {
	c := &Collection{}
	err := d.QueryRow(
		`SELECT id, workspace_id, name, description, color, sort_order, created_at, updated_at
		 FROM collections WHERE id=?`, id,
	).Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Description, &c.Color, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	reqs, err := d.listSavedRequests(id)
	if err != nil {
		return nil, err
	}
	c.Requests = reqs
	return c, nil
}

func (d *DB) UpdateCollection(id, name, description, color string) error {
	_, err := d.Exec(
		`UPDATE collections SET name=?, description=?, color=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		name, description, color, id,
	)
	return err
}

func (d *DB) DeleteCollection(id string) error {
	_, err := d.Exec(`DELETE FROM collections WHERE id=?`, id)
	return err
}

func (d *DB) MigrateOrphanCollections(workspaceID string) (int64, error) {
	res, err := d.Exec(
		`UPDATE collections SET workspace_id=? WHERE workspace_id IS NULL`,
		workspaceID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ── Saved Requests ────────────────────────────────────────────────────────────

func (d *DB) listSavedRequests(collectionID string) ([]*SavedRequest, error) {
	rows, err := d.Query(
		`SELECT id, collection_id, name, description, service, method, body, metadata,
		        conn_address, env_id, sort_order, created_at, updated_at
		 FROM saved_requests WHERE collection_id=? ORDER BY sort_order ASC, created_at ASC`,
		collectionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []*SavedRequest
	for rows.Next() {
		r := &SavedRequest{}
		var metaJSON string
		var envID sql.NullString
		if err := rows.Scan(
			&r.ID, &r.CollectionID, &r.Name, &r.Description,
			&r.Service, &r.Method, &r.Body, &metaJSON,
			&r.ConnAddress, &envID, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if envID.Valid {
			r.EnvID = &envID.String
		}
		if err := json.Unmarshal([]byte(metaJSON), &r.Metadata); err != nil {
			r.Metadata = map[string]string{}
		}
		reqs = append(reqs, r)
	}
	if reqs == nil {
		reqs = []*SavedRequest{}
	}
	return reqs, nil
}

func (d *DB) SaveRequest(req *SavedRequest) (*SavedRequest, error) {
	metaJSON, _ := json.Marshal(req.Metadata)
	var envID interface{}
	if req.EnvID != nil && *req.EnvID != "" {
		envID = *req.EnvID
	}

	_, err := d.Exec(`
		INSERT INTO saved_requests
		  (id, collection_id, name, description, service, method, body, metadata, conn_address, env_id, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name=excluded.name, description=excluded.description,
		  service=excluded.service, method=excluded.method,
		  body=excluded.body, metadata=excluded.metadata,
		  conn_address=excluded.conn_address, env_id=excluded.env_id,
		  updated_at=CURRENT_TIMESTAMP`,
		req.ID, req.CollectionID, req.Name, req.Description,
		req.Service, req.Method, req.Body, string(metaJSON),
		req.ConnAddress, envID, req.SortOrder,
	)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (d *DB) DeleteSavedRequest(id string) error {
	_, err := d.Exec(`DELETE FROM saved_requests WHERE id=?`, id)
	return err
}

// GetSavedRequest fetches a single saved request with its collection's workspace_id.
func (d *DB) GetSavedRequest(id string) (*SavedRequest, error) {
	r := &SavedRequest{}
	var metaJSON string
	var envID sql.NullString
	err := d.QueryRow(`
		SELECT sr.id, sr.collection_id, sr.name, sr.description,
		       sr.service, sr.method, sr.body, sr.metadata,
		       sr.conn_address, sr.env_id, sr.sort_order, sr.created_at, sr.updated_at
		FROM saved_requests sr WHERE sr.id=?`, id,
	).Scan(
		&r.ID, &r.CollectionID, &r.Name, &r.Description,
		&r.Service, &r.Method, &r.Body, &metaJSON,
		&r.ConnAddress, &envID, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if envID.Valid {
		r.EnvID = &envID.String
	}
	if err := json.Unmarshal([]byte(metaJSON), &r.Metadata); err != nil {
		r.Metadata = map[string]string{}
	}
	return r, nil
}

// WorkspaceIDForCollection returns the workspace_id of a collection.
func (d *DB) WorkspaceIDForCollection(collectionID string) string {
	var wsID string
	d.QueryRow(`SELECT workspace_id FROM collections WHERE id=?`, collectionID).Scan(&wsID)
	return wsID
}

func (d *DB) MoveRequest(requestID, targetCollectionID string) error {
	_, err := d.Exec(
		`UPDATE saved_requests SET collection_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		targetCollectionID, requestID,
	)
	return err
}
