package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

type HistoryEntry struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspaceId"`
	UserID       string          `json:"userId,omitempty"`
	ConnID       string          `json:"connectionId"`
	ConnAddress  string          `json:"connAddress"`
	Service      string          `json:"service"`
	Method       string          `json:"method"`
	RequestBody  json.RawMessage `json:"requestBody"`
	ResponseBody json.RawMessage `json:"responseBody,omitempty"`
	Status       string          `json:"status"`
	DurationMs   int64           `json:"durationMs"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (d *DB) AddHistoryEntry(e *HistoryEntry) error {
	reqBody, _ := json.Marshal(e.RequestBody)
	resBody, _ := json.Marshal(e.ResponseBody)
	_, err := d.Exec(`
		INSERT INTO request_history
		  (id, workspace_id, user_id, conn_id, conn_address, service, method,
		   request_body, response_body, status, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.WorkspaceID, nullStr(e.UserID), e.ConnID, e.ConnAddress,
		e.Service, e.Method, string(reqBody), string(resBody),
		e.Status, e.DurationMs,
	)
	return err
}

func (d *DB) ListHistory(workspaceID string, limit int) ([]*HistoryEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := d.Query(`
		SELECT id, workspace_id, COALESCE(user_id,''), conn_id, conn_address,
		       service, method, request_body, response_body, status, duration_ms, created_at
		FROM request_history
		WHERE workspace_id=?
		ORDER BY created_at DESC
		LIMIT ?`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*HistoryEntry
	for rows.Next() {
		e := &HistoryEntry{}
		var reqBody, resBody string
		if err := rows.Scan(
			&e.ID, &e.WorkspaceID, &e.UserID, &e.ConnID, &e.ConnAddress,
			&e.Service, &e.Method, &reqBody, &resBody,
			&e.Status, &e.DurationMs, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		e.RequestBody = json.RawMessage(reqBody)
		e.ResponseBody = json.RawMessage(resBody)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []*HistoryEntry{}
	}
	return entries, nil
}

func (d *DB) DeleteHistoryEntry(id, workspaceID string) error {
	_, err := d.Exec(`DELETE FROM request_history WHERE id=? AND workspace_id=?`, id, workspaceID)
	return err
}

func (d *DB) ClearHistory(workspaceID string) error {
	_, err := d.Exec(`DELETE FROM request_history WHERE workspace_id=?`, workspaceID)
	return err
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// GetHistoryEntry fetches a single entry (ownership check via workspaceID).
func (d *DB) GetHistoryEntry(id, workspaceID string) (*HistoryEntry, error) {
	e := &HistoryEntry{}
	var reqBody, resBody string
	err := d.QueryRow(`
		SELECT id, workspace_id, COALESCE(user_id,''), conn_id, conn_address,
		       service, method, request_body, response_body, status, duration_ms, created_at
		FROM request_history WHERE id=? AND workspace_id=?`, id, workspaceID,
	).Scan(
		&e.ID, &e.WorkspaceID, &e.UserID, &e.ConnID, &e.ConnAddress,
		&e.Service, &e.Method, &reqBody, &resBody,
		&e.Status, &e.DurationMs, &e.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.RequestBody = json.RawMessage(reqBody)
	e.ResponseBody = json.RawMessage(resBody)
	return e, nil
}
