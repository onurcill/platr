package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the sql.DB with migration support.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*DB, error) {
	if path == "" {
		path = os.Getenv("DB_PATH")
	}
	if path == "" {
		path = "./grpc-inspector.db"
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := path + "?_journal=WAL&_timeout=10000&_fk=true&_mutex=full"
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)

	d := &DB{sqldb}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	log.Printf("💾 Database: %s", path)
	return d, nil
}

func (d *DB) migrate() error {
	// ── Core schema ────────────────────────────────────────────────────────────
	_, err := d.Exec(`
	CREATE TABLE IF NOT EXISTS users (
		id            TEXT PRIMARY KEY,
		email         TEXT NOT NULL UNIQUE,
		name          TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS workspaces (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		owner_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS workspace_members (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role         TEXT NOT NULL DEFAULT 'viewer',
		joined_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, user_id)
	);

	CREATE TABLE IF NOT EXISTS workspace_invites (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		email        TEXT NOT NULL,
		role         TEXT NOT NULL DEFAULT 'editor',
		token        TEXT NOT NULL UNIQUE,
		created_by   TEXT NOT NULL REFERENCES users(id),
		expires_at   DATETIME NOT NULL,
		used         INTEGER NOT NULL DEFAULT 0,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS roles (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL,
		description  TEXT NOT NULL DEFAULT '',
		rank         INTEGER NOT NULL DEFAULT 0,
		is_system    INTEGER NOT NULL DEFAULT 1,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS role_permissions (
		id         TEXT PRIMARY KEY,
		role_id    TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
		action     TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(role_id, action)
	);

	CREATE TABLE IF NOT EXISTS environments (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		name         TEXT NOT NULL,
		color        TEXT NOT NULL DEFAULT '#818cf8',
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS env_variables (
		id         TEXT PRIMARY KEY,
		env_id     TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
		key        TEXT NOT NULL,
		value      TEXT NOT NULL DEFAULT '',
		secret     INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS proto_files (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		conn_address TEXT NOT NULL DEFAULT '',
		filename     TEXT NOT NULL,
		data         BLOB NOT NULL,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS request_history (
		id            TEXT PRIMARY KEY,
		workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		user_id       TEXT REFERENCES users(id) ON DELETE SET NULL,
		conn_id       TEXT NOT NULL DEFAULT '',
		conn_address  TEXT NOT NULL DEFAULT '',
		service       TEXT NOT NULL,
		method        TEXT NOT NULL,
		request_body  TEXT,
		response_body TEXT,
		status        TEXT,
		duration_ms   INTEGER,
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS collections (
		id           TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		name         TEXT NOT NULL,
		description  TEXT NOT NULL DEFAULT '',
		color        TEXT NOT NULL DEFAULT '#818cf8',
		sort_order   INTEGER NOT NULL DEFAULT 0,
		created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS saved_requests (
		id            TEXT PRIMARY KEY,
		collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
		name          TEXT NOT NULL,
		description   TEXT NOT NULL DEFAULT '',
		service       TEXT NOT NULL,
		method        TEXT NOT NULL,
		body          TEXT NOT NULL DEFAULT '{}',
		metadata      TEXT NOT NULL DEFAULT '{}',
		conn_address  TEXT NOT NULL DEFAULT '',
		env_id        TEXT REFERENCES environments(id) ON DELETE SET NULL,
		sort_order    INTEGER NOT NULL DEFAULT 0,
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	);


	CREATE TABLE IF NOT EXISTS subscriptions (
		id                     TEXT PRIMARY KEY,
		user_id                TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
		plan                   TEXT NOT NULL DEFAULT 'free',
		status                 TEXT NOT NULL DEFAULT 'active',
		stripe_customer_id     TEXT,
		stripe_subscription_id TEXT,
		stripe_price_id        TEXT,
		current_period_start   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		current_period_end     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		cancel_at_period_end   INTEGER NOT NULL DEFAULT 0,
		created_at             DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at             DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS usage_records (
		id             TEXT PRIMARY KEY DEFAULT (hex(randomblob(8))),
		user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		period_month   TEXT NOT NULL,  -- format: '2006-01'
		invocations    INTEGER NOT NULL DEFAULT 0,
		created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, period_month)
	);

	CREATE TABLE IF NOT EXISTS request_assertions (
		id          TEXT PRIMARY KEY,
		request_id  TEXT NOT NULL REFERENCES saved_requests(id) ON DELETE CASCADE,
		name        TEXT NOT NULL DEFAULT '',
		target      TEXT NOT NULL DEFAULT 'status',
		path        TEXT NOT NULL DEFAULT '',
		operator    TEXT NOT NULL DEFAULT 'equals',
		value       TEXT NOT NULL DEFAULT '',
		enabled     INTEGER NOT NULL DEFAULT 1,
		sort_order  INTEGER NOT NULL DEFAULT 0,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS request_extractions (
		id            TEXT PRIMARY KEY,
		request_id    TEXT NOT NULL REFERENCES saved_requests(id) ON DELETE CASCADE,
		variable_name TEXT NOT NULL,
		json_path     TEXT NOT NULL,
		enabled       INTEGER NOT NULL DEFAULT 1,
		sort_order    INTEGER NOT NULL DEFAULT 0,
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS test_runs (
		id            TEXT PRIMARY KEY,
		collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
		workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
		user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		status        TEXT NOT NULL DEFAULT 'running',
		results_json  TEXT NOT NULL DEFAULT '[]',
		pass_count    INTEGER NOT NULL DEFAULT 0,
		fail_count    INTEGER NOT NULL DEFAULT 0,
		total_ms      INTEGER NOT NULL DEFAULT 0,
		started_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		finished_at   DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_assertions_request   ON request_assertions(request_id);
	CREATE INDEX IF NOT EXISTS idx_extractions_request  ON request_extractions(request_id);
	CREATE INDEX IF NOT EXISTS idx_test_runs_collection ON test_runs(collection_id);
	CREATE INDEX IF NOT EXISTS idx_test_runs_workspace  ON test_runs(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_test_runs_user       ON test_runs(user_id);

	CREATE INDEX IF NOT EXISTS idx_subscriptions_user   ON subscriptions(user_id);
	CREATE INDEX IF NOT EXISTS idx_subscriptions_stripe ON subscriptions(stripe_subscription_id);
	CREATE INDEX IF NOT EXISTS idx_usage_user_month     ON usage_records(user_id, period_month);
	`)
	if err != nil {
		return err
	}

	// ── Additive migrations — safe to re-run on existing databases ─────────────
	// IMPORTANT: SQLite ALTER TABLE does not support REFERENCES constraints.
	// Add columns as plain TEXT only — FK enforcement is at the app layer.
	migrations := []string{
		`ALTER TABLE environments    ADD COLUMN workspace_id TEXT`,
		`ALTER TABLE proto_files     ADD COLUMN workspace_id TEXT`,
		`ALTER TABLE proto_files     ADD COLUMN conn_address TEXT DEFAULT ''`,
		`ALTER TABLE request_history ADD COLUMN workspace_id TEXT`,
		`ALTER TABLE request_history ADD COLUMN user_id      TEXT`,
		`ALTER TABLE request_history ADD COLUMN conn_address TEXT DEFAULT ''`,
		`ALTER TABLE collections     ADD COLUMN workspace_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_history_workspace  ON request_history(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_history_user       ON request_history(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_collections_ws     ON collections(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_proto_ws           ON proto_files(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_env_ws             ON environments(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_saved_req_col      ON saved_requests(collection_id)`,
		// test_runs: backfill workspace_id from parent collection (for pre-FK rows)
		`UPDATE test_runs SET workspace_id = (SELECT workspace_id FROM collections WHERE collections.id = test_runs.collection_id) WHERE workspace_id = '' OR workspace_id IS NULL`,
		// Additional indexes added in v2
		`CREATE INDEX IF NOT EXISTS idx_test_runs_workspace ON test_runs(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_test_runs_user      ON test_runs(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_history_created     ON request_history(workspace_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_members_user        ON workspace_members(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invites_workspace   ON workspace_invites(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invites_token       ON workspace_invites(token)`,
	}
	for _, m := range migrations {
		d.Exec(m) // ignore errors: column/index already exists
	}

	// ── Data integrity fixes ───────────────────────────────────────────────────
	// Ensure every workspace owner is in workspace_members
	if _, err := d.Exec(`
		INSERT OR IGNORE INTO workspace_members (id, workspace_id, user_id, role)
		SELECT hex(randomblob(8)), w.id, w.owner_id, 'owner'
		FROM workspaces w
		WHERE NOT EXISTS (
			SELECT 1 FROM workspace_members wm
			WHERE wm.workspace_id = w.id AND wm.user_id = w.owner_id
		)
	`); err != nil {
		return err
	}

	// Migrate orphan collections/environments to first available workspace
	if _, err := d.Exec(`
		UPDATE collections SET workspace_id = (SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1)
		WHERE workspace_id IS NULL AND EXISTS (SELECT 1 FROM workspaces LIMIT 1)
	`); err != nil {
		return err
	}
	if _, err := d.Exec(`
		UPDATE environments SET workspace_id = (SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1)
		WHERE workspace_id IS NULL AND EXISTS (SELECT 1 FROM workspaces LIMIT 1)
	`); err != nil {
		return err
	}
	if _, err := d.Exec(`
		UPDATE proto_files SET workspace_id = (SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1)
		WHERE workspace_id IS NULL AND EXISTS (SELECT 1 FROM workspaces LIMIT 1)
	`); err != nil {
		return err
	}
	if _, err := d.Exec(`
		UPDATE request_history SET workspace_id = (SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1)
		WHERE workspace_id IS NULL AND EXISTS (SELECT 1 FROM workspaces LIMIT 1)
	`); err != nil {
		return err
	}

	return d.seedRoles()
}

func (d *DB) seedRoles() error {
	roles := []struct {
		id, name, displayName, description string
		rank                               int
	}{
		{"role_owner",  "owner",  "Owner",  "Full control. Can delete workspace.", 4},
		{"role_admin",  "admin",  "Admin",  "Can manage members and invites.",      3},
		{"role_editor", "editor", "Editor", "Can create collections and protos.",   2},
		{"role_viewer", "viewer", "Viewer", "Can send requests and view history.",  1},
		{"role_guest",  "guest",  "Guest",  "Read-only access to collections.",     0},
	}

	for _, r := range roles {
		if _, err := d.Exec(`
			INSERT OR IGNORE INTO roles (id, name, display_name, description, rank, is_system)
			VALUES (?, ?, ?, ?, ?, 1)`,
			r.id, r.name, r.displayName, r.description, r.rank,
		); err != nil {
			return err
		}
	}

	matrix := map[string][]string{
		"role_owner": {
			"workspace:update", "workspace:delete",
			"member:invite", "member:remove", "member:set_role",
			"collection:create", "collection:update", "collection:delete", "collection:read",
			"request:create", "request:update", "request:delete",
			"invoke", "environment:manage", "environment:read",
			"proto:upload", "history:read",
		},
		"role_admin": {
			"member:invite", "member:remove", "member:set_role",
			"collection:create", "collection:update", "collection:delete", "collection:read",
			"request:create", "request:update", "request:delete",
			"invoke", "environment:manage", "environment:read",
			"proto:upload", "history:read",
		},
		"role_editor": {
			"collection:create", "collection:update", "collection:delete", "collection:read",
			"request:create", "request:update", "request:delete",
			"invoke", "environment:manage", "environment:read",
			"proto:upload", "history:read",
		},
		"role_viewer": {
			"collection:read", "invoke", "environment:read", "history:read",
		},
		"role_guest": {
			"collection:read",
		},
	}

	for roleID, actions := range matrix {
		for _, action := range actions {
			if _, err := d.Exec(`
				INSERT OR IGNORE INTO role_permissions (id, role_id, action)
				VALUES (?, ?, ?)`,
				roleID+"_"+action, roleID, action,
			); err != nil {
				return err
			}
		}
	}
	return nil
}
