-- Additive integrations schema (also applied by hosted.BootstrapHostedSchema).

CREATE TABLE IF NOT EXISTS integration_credentials (
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
);

CREATE TABLE IF NOT EXISTS enrichment_calls (
  id BIGSERIAL PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'default',
  provider TEXT NOT NULL,
  units DOUBLE PRECISION NOT NULL DEFAULT 1,
  operation TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS microsoft_credentials (
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
);

CREATE TABLE IF NOT EXISTS webhook_endpoints (
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
);
