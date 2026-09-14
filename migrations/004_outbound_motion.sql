-- Additive tables for shortlist review, dependable handoff, and conversation ownership.
-- Also applied by hosted.BootstrapHostedSchema.

CREATE TABLE IF NOT EXISTS shortlist_candidates (
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
);

CREATE INDEX IF NOT EXISTS shortlist_ws_status ON shortlist_candidates(workspace_id, status);

CREATE TABLE IF NOT EXISTS outbound_deliveries (
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
);

CREATE INDEX IF NOT EXISTS outbound_deliveries_ws_status ON outbound_deliveries(workspace_id, status);

CREATE TABLE IF NOT EXISTS conversation_state (
  campaign_id BIGINT NOT NULL,
  lead_id BIGINT NOT NULL,
  owner_email TEXT NOT NULL DEFAULT '',
  needs_action INTEGER NOT NULL DEFAULT 0,
  handoff_status TEXT NOT NULL DEFAULT 'none',
  last_delivery_id BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (campaign_id, lead_id)
);
