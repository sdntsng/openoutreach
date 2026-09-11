CREATE TABLE IF NOT EXISTS workspace_members (
  workspace_id TEXT NOT NULL,
  email TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'member',
  status TEXT NOT NULL DEFAULT 'invited',
  invite_token TEXT,
  invited_by TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  accepted_at TEXT,
  PRIMARY KEY (workspace_id, email)
);

CREATE UNIQUE INDEX IF NOT EXISTS workspace_members_invite_token
  ON workspace_members (invite_token);
