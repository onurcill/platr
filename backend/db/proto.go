package db

// ProtoFile represents a stored proto descriptor for a workspace+address pair.
type ProtoFile struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	ConnAddress string `json:"connAddress"`
	Filename    string `json:"filename"`
	Data        []byte `json:"-"`
}

// SaveProtoFiles replaces all proto files for a workspace+address pair.
func (d *DB) SaveProtoFiles(workspaceID, connAddress string, files []ProtoFile) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM proto_files WHERE workspace_id=? AND conn_address=?`,
		workspaceID, connAddress,
	); err != nil {
		return err
	}
	for _, f := range files {
		if _, err := tx.Exec(
			`INSERT INTO proto_files (id, workspace_id, conn_address, filename, data) VALUES (?, ?, ?, ?, ?)`,
			f.ID, workspaceID, connAddress, f.Filename, f.Data,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadProtoFiles loads all proto files for a workspace+address pair.
func (d *DB) LoadProtoFiles(workspaceID, connAddress string) ([]ProtoFile, error) {
	rows, err := d.Query(
		`SELECT id, workspace_id, conn_address, filename, data
		 FROM proto_files WHERE workspace_id=? AND conn_address=? ORDER BY created_at ASC`,
		workspaceID, connAddress,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []ProtoFile
	for rows.Next() {
		var f ProtoFile
		if err := rows.Scan(&f.ID, &f.WorkspaceID, &f.ConnAddress, &f.Filename, &f.Data); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// LoadAllProtoFilesForWorkspace loads all proto files for a workspace grouped by address.
func (d *DB) LoadAllProtoFilesForWorkspace(workspaceID string) (map[string][]ProtoFile, error) {
	rows, err := d.Query(
		`SELECT id, workspace_id, conn_address, filename, data
		 FROM proto_files WHERE workspace_id=? ORDER BY conn_address, created_at ASC`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string][]ProtoFile{}
	for rows.Next() {
		var f ProtoFile
		if err := rows.Scan(&f.ID, &f.WorkspaceID, &f.ConnAddress, &f.Filename, &f.Data); err != nil {
			return nil, err
		}
		result[f.ConnAddress] = append(result[f.ConnAddress], f)
	}
	return result, nil
}

// DeleteProtoFiles removes all proto files for a workspace+address pair.
func (d *DB) DeleteProtoFiles(workspaceID, connAddress string) error {
	_, err := d.Exec(
		`DELETE FROM proto_files WHERE workspace_id=? AND conn_address=?`,
		workspaceID, connAddress,
	)
	return err
}
