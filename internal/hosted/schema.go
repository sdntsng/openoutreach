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
	`CREATE TABLE IF NOT EXISTS shortlist_candidates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		campaign_id INTEGER NOT NULL DEFAULT 0,
		email TEXT NOT NULL,
		first_name TEXT NOT NULL DEFAULT '',
		last_name TEXT NOT NULL DEFAULT '',
		company TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		domain TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'csv',
		source_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		fit_reason TEXT NOT NULL DEFAULT '',
		email_check TEXT NOT NULL DEFAULT 'unknown',
		email_check_reason TEXT NOT NULL DEFAULT '',
		previous_outreach TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, campaign_id, email)
	)`,
	`CREATE INDEX IF NOT EXISTS shortlist_ws_status ON shortlist_candidates(workspace_id, status)`,
	`CREATE TABLE IF NOT EXISTS outbound_deliveries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		event_id INTEGER NOT NULL DEFAULT 0,
		kind TEXT NOT NULL,
		campaign_id INTEGER NOT NULL DEFAULT 0,
		lead_id INTEGER NOT NULL DEFAULT 0,
		payload TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'pending',
		http_status INTEGER NOT NULL DEFAULT 0,
		error_message TEXT NOT NULL DEFAULT '',
		attempts INTEGER NOT NULL DEFAULT 0,
		last_attempt_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(workspace_id, event_id, kind, campaign_id, lead_id)
	)`,
	`CREATE INDEX IF NOT EXISTS outbound_deliveries_ws_status ON outbound_deliveries(workspace_id, status)`,
	`CREATE TABLE IF NOT EXISTS conversation_state (
		campaign_id INTEGER NOT NULL,
		lead_id INTEGER NOT NULL,
		owner_email TEXT NOT NULL DEFAULT '',
		needs_action INTEGER NOT NULL DEFAULT 0,
		handoff_status TEXT NOT NULL DEFAULT 'none',
		last_delivery_id INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (campaign_id, lead_id)
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
	`CREATE TABLE IF NOT EXISTS shortlist_candidates (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		campaign_id BIGINT NOT NULL DEFAULT 0,
		email TEXT NOT NULL,
		first_name TEXT NOT NULL DEFAULT '',
		last_name TEXT NOT NULL DEFAULT '',
		company TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		domain TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'csv',
		source_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		fit_reason TEXT NOT NULL DEFAULT '',
		email_check TEXT NOT NULL DEFAULT 'unknown',
		email_check_reason TEXT NOT NULL DEFAULT '',
		previous_outreach TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(workspace_id, campaign_id, email)
	)`,
	`CREATE INDEX IF NOT EXISTS shortlist_ws_status ON shortlist_candidates(workspace_id, status)`,
	`CREATE TABLE IF NOT EXISTS outbound_deliveries (
		id BIGSERIAL PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT 'default',
		event_id BIGINT NOT NULL DEFAULT 0,
		kind TEXT NOT NULL,
		campaign_id BIGINT NOT NULL DEFAULT 0,
		lead_id BIGINT NOT NULL DEFAULT 0,
		payload TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'pending',
		http_status INTEGER NOT NULL DEFAULT 0,
		error_message TEXT NOT NULL DEFAULT '',
		attempts INTEGER NOT NULL DEFAULT 0,
		last_attempt_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(workspace_id, event_id, kind, campaign_id, lead_id)
	)`,
	`CREATE INDEX IF NOT EXISTS outbound_deliveries_ws_status ON outbound_deliveries(workspace_id, status)`,
	`CREATE TABLE IF NOT EXISTS conversation_state (
		campaign_id BIGINT NOT NULL,
		lead_id BIGINT NOT NULL,
		owner_email TEXT NOT NULL DEFAULT '',
		needs_action INTEGER NOT NULL DEFAULT 0,
		handoff_status TEXT NOT NULL DEFAULT 'none',
		last_delivery_id BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (campaign_id, lead_id)
	)`,
}
