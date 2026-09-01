package hosted

import (
	"database/sql"
	"fmt"

	"github.com/andersmyrmel/cold-cli/internal"
)

// BootstrapHostedSchema adds OpenOutreach tables on top of cold-cli schema.
func BootstrapHostedSchema(db *sql.DB) error {
	dialect := internal.CurrentDialect()
	stmts := hostedSchemaSQLite
	if dialect == internal.DialectPostgres {
		stmts = hostedSchemaPostgres
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("hosted schema: %w\nstmt: %s", err, stmt)
		}
	}
	return nil
}

var hostedSchemaSQLite = []string{
	`CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`INSERT OR IGNORE INTO workspaces (id, name) VALUES ('default', 'Default')`,
	`CREATE TABLE IF NOT EXISTS google_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		account_id INTEGER NOT NULL UNIQUE,
		google_account_id TEXT NOT NULL DEFAULT '',
		encrypted_refresh_token TEXT NOT NULL,
		encrypted_access_token TEXT NOT NULL DEFAULT '',
		token_expiry DATETIME,
		scopes TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS oauth_states (
		state TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS tracking_tokens (
		token TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		campaign_id INTEGER NOT NULL,
		lead_id INTEGER NOT NULL,
		account_id INTEGER NOT NULL,
		scheduled_send_id INTEGER,
		message_id TEXT NOT NULL DEFAULT '',
		destination_url TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS reply_classifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		campaign_id INTEGER NOT NULL,
		lead_id INTEGER NOT NULL,
		email_message_id INTEGER,
		classification TEXT NOT NULL,
		confidence REAL NOT NULL DEFAULT 0,
		reason TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS hosted_kv (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tick_lock (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		holder TEXT NOT NULL,
		locked_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS integration_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL,
		name TEXT NOT NULL,
		encrypted_secret TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, provider, name)
	)`,
	`CREATE TABLE IF NOT EXISTS enrichment_calls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL,
		units REAL NOT NULL DEFAULT 1,
		operation TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS microsoft_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		account_id INTEGER NOT NULL UNIQUE,
		microsoft_account_id TEXT NOT NULL DEFAULT '',
		encrypted_refresh_token TEXT NOT NULL,
		encrypted_access_token TEXT NOT NULL DEFAULT '',
		token_expiry DATETIME,
		scopes TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS webhook_endpoints (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL DEFAULT 'generic',
		name TEXT NOT NULL,
		encrypted_hmac_secret TEXT NOT NULL DEFAULT '',
		campaign_id INTEGER,
		field_map TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, provider, name)
	)`,
	`CREATE TABLE IF NOT EXISTS webhook_idempotency (
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (workspace_id, provider, idempotency_key)
	)`,
	`CREATE TABLE IF NOT EXISTS suppressions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		kind TEXT NOT NULL,
		value TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, kind, value)
	)`,
}

var hostedSchemaPostgres = []string{
	`CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`INSERT INTO workspaces (id, name) VALUES ('default', 'Default') ON CONFLICT (id) DO NOTHING`,
	`CREATE TABLE IF NOT EXISTS google_credentials (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		account_id BIGINT NOT NULL UNIQUE,
		google_account_id TEXT NOT NULL DEFAULT '',
		encrypted_refresh_token TEXT NOT NULL,
		encrypted_access_token TEXT NOT NULL DEFAULT '',
		token_expiry TIMESTAMPTZ,
		scopes TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS oauth_states (
		state TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS tracking_tokens (
		token TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		campaign_id BIGINT NOT NULL,
		lead_id BIGINT NOT NULL,
		account_id BIGINT NOT NULL,
		scheduled_send_id BIGINT,
		message_id TEXT NOT NULL DEFAULT '',
		destination_url TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS reply_classifications (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		campaign_id BIGINT NOT NULL,
		lead_id BIGINT NOT NULL,
		email_message_id BIGINT,
		classification TEXT NOT NULL,
		confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
		reason TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS hosted_kv (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS integration_credentials (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL,
		name TEXT NOT NULL,
		encrypted_secret TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(workspace_id, provider, name)
	)`,
	`CREATE TABLE IF NOT EXISTS enrichment_calls (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL,
		units DOUBLE PRECISION NOT NULL DEFAULT 1,
		operation TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS microsoft_credentials (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		account_id BIGINT NOT NULL UNIQUE,
		microsoft_account_id TEXT NOT NULL DEFAULT '',
		encrypted_refresh_token TEXT NOT NULL,
		encrypted_access_token TEXT NOT NULL DEFAULT '',
		token_expiry TIMESTAMPTZ,
		scopes TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS webhook_endpoints (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL DEFAULT 'generic',
		name TEXT NOT NULL,
		encrypted_hmac_secret TEXT NOT NULL DEFAULT '',
		campaign_id BIGINT,
		field_map TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(workspace_id, provider, name)
	)`,
	`CREATE TABLE IF NOT EXISTS webhook_idempotency (
		workspace_id TEXT NOT NULL DEFAULT 'default',
		provider TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (workspace_id, provider, idempotency_key)
	)`,
	`CREATE TABLE IF NOT EXISTS suppressions (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		kind TEXT NOT NULL,
		value TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(workspace_id, kind, value)
	)`,
}
