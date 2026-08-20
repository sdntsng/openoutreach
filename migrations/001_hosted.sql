-- Additive hosted schema (also applied by hosted.BootstrapHostedSchema).
-- Prefer running outreachd once against the target DB; this file documents the tables.

CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS google_credentials (
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
);

CREATE TABLE IF NOT EXISTS oauth_states (
  state TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tracking_tokens (
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
);

CREATE TABLE IF NOT EXISTS reply_classifications (
  id BIGSERIAL PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'default',
  campaign_id BIGINT NOT NULL,
  lead_id BIGINT NOT NULL,
  email_message_id BIGINT,
  classification TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS hosted_kv (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
