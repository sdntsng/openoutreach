import type { AuthEnv } from "./auth";

export type MemberRole = "owner" | "member";
export type MemberStatus = "invited" | "active" | "revoked";

export interface WorkspaceMember {
  email: string;
  role: MemberRole;
  status: MemberStatus;
  invited_by?: string;
  created_at?: string;
  accepted_at?: string;
  invite_url?: string;
}

export function workspaceID(env: { OPENOUTREACH_WORKSPACE_ID?: string }): string {
  return (env.OPENOUTREACH_WORKSPACE_ID || "default").trim() || "default";
}

export function allowlist(env: { AUTH_ALLOWED_EMAILS?: string }): string[] {
  return (env.AUTH_ALLOWED_EMAILS || "")
    .split(",")
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
}

export function normalizeEmail(email: string): string {
  return email.trim().toLowerCase();
}

export function isAllowlisted(env: { AUTH_ALLOWED_EMAILS?: string }, email: string): boolean {
  const allow = allowlist(env);
  if (allow.length === 0) return false;
  return allow.includes(normalizeEmail(email));
}

export async function emailMayJoin(env: AuthEnv, email: string): Promise<boolean> {
  const e = normalizeEmail(email);
  if (!e || !e.includes("@")) return false;
  const allow = allowlist(env);
  if (allow.length === 0) return true;
  if (allow.includes(e)) return true;
  if (!env.DB) return false;
  const row = await env.DB.prepare(
    `SELECT email FROM workspace_members
     WHERE workspace_id = ? AND email = ? AND status IN ('invited', 'active')`,
  )
    .bind(workspaceID(env), e)
    .first();
  return Boolean(row);
}

export function isOwnerEmail(env: AuthEnv, email: string, members: WorkspaceMember[]): boolean {
  const e = normalizeEmail(email);
  if (isAllowlisted(env, e)) return true;
  return members.some((m) => m.email === e && m.role === "owner" && m.status !== "revoked");
}

function newToken(): string {
  const bytes = new Uint8Array(18);
  crypto.getRandomValues(bytes);
  return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function inviteURL(env: AuthEnv, request: Request, token: string): string {
  const base = (env.PUBLIC_BASE_URL || new URL(request.url).origin).replace(/\/$/, "");
  return `${base}/sign-up?invite=${encodeURIComponent(token)}`;
}

export async function listMembers(env: AuthEnv): Promise<WorkspaceMember[]> {
  if (!env.DB) return [];
  const ws = workspaceID(env);
  const res = await env.DB.prepare(
    `SELECT email, role, status, invited_by, created_at, accepted_at, invite_token
     FROM workspace_members WHERE workspace_id = ? ORDER BY role DESC, email`,
  )
    .bind(ws)
    .all<{
      email: string;
      role: MemberRole;
      status: MemberStatus;
      invited_by?: string;
      created_at?: string;
      accepted_at?: string;
      invite_token?: string;
    }>();
  return (res.results || []).map((row) => ({
    email: row.email,
    role: row.role,
    status: row.status,
    invited_by: row.invited_by || undefined,
    created_at: row.created_at,
    accepted_at: row.accepted_at || undefined,
  }));
}

export async function ensureMember(
  env: AuthEnv,
  email: string,
  role: MemberRole,
  status: MemberStatus,
): Promise<void> {
  if (!env.DB) return;
  const e = normalizeEmail(email);
  const ws = workspaceID(env);
  await env.DB.prepare(
    `INSERT INTO workspace_members (workspace_id, email, role, status, accepted_at)
     VALUES (?, ?, ?, ?, CASE WHEN ? = 'active' THEN datetime('now') ELSE NULL END)
     ON CONFLICT (workspace_id, email) DO UPDATE SET
       role = CASE WHEN workspace_members.role = 'owner' THEN workspace_members.role ELSE excluded.role END,
       status = CASE
         WHEN workspace_members.status = 'revoked' AND excluded.status = 'invited' THEN 'invited'
         WHEN excluded.status = 'active' THEN 'active'
         ELSE workspace_members.status
       END,
       accepted_at = CASE
         WHEN excluded.status = 'active' THEN COALESCE(workspace_members.accepted_at, datetime('now'))
         ELSE workspace_members.accepted_at
       END`,
  )
    .bind(ws, e, role, status, status)
    .run();
}

export async function seedCaller(env: AuthEnv, email: string): Promise<void> {
  const e = normalizeEmail(email);
  if (!e) return;
  const role: MemberRole = isAllowlisted(env, e) || allowlist(env).length === 0 ? "owner" : "member";
  const members = await listMembers(env);
  if (members.length === 0 && (role === "owner" || allowlist(env).length === 0)) {
    await ensureMember(env, e, "owner", "active");
    return;
  }
  if (isAllowlisted(env, e)) {
    await ensureMember(env, e, "owner", "active");
  }
}

export async function createInvite(
  env: AuthEnv,
  request: Request,
  actorEmail: string,
  rawEmail: string,
): Promise<{ member: WorkspaceMember; error?: string; status?: number }> {
  const email = normalizeEmail(rawEmail);
  if (!email.includes("@")) {
    return { member: { email, role: "member", status: "invited" }, error: "email is required", status: 400 };
  }
  await seedCaller(env, actorEmail);
  const members = await listMembers(env);
  if (!isOwnerEmail(env, actorEmail, members)) {
    return { member: { email, role: "member", status: "invited" }, error: "only owners can invite", status: 403 };
  }
  if (!env.DB) {
    return { member: { email, role: "member", status: "invited" }, error: "D1 is required for invites", status: 501 };
  }
  const existing = members.find((m) => m.email === email);
  if (existing?.status === "active") {
    return { member: existing, error: "already a member", status: 409 };
  }
  const token = newToken();
  const ws = workspaceID(env);
  await env.DB.prepare(
    `INSERT INTO workspace_members (workspace_id, email, role, status, invite_token, invited_by)
     VALUES (?, ?, 'member', 'invited', ?, ?)
     ON CONFLICT (workspace_id, email) DO UPDATE SET
       status = 'invited',
       invite_token = excluded.invite_token,
       invited_by = excluded.invited_by,
       role = workspace_members.role`,
  )
    .bind(ws, email, token, normalizeEmail(actorEmail))
    .run();
  return {
    member: {
      email,
      role: existing?.role === "owner" ? "owner" : "member",
      status: "invited",
      invited_by: normalizeEmail(actorEmail),
      invite_url: inviteURL(env, request, token),
    },
  };
}

export async function lookupInvite(env: AuthEnv, token: string): Promise<WorkspaceMember | null> {
  if (!env.DB || !token.trim()) return null;
  const row = await env.DB.prepare(
    `SELECT email, role, status FROM workspace_members
     WHERE invite_token = ? AND status = 'invited'`,
  )
    .bind(token.trim())
    .first<{ email: string; role: MemberRole; status: MemberStatus }>();
  if (!row) return null;
  return { email: row.email, role: row.role, status: row.status };
}

export async function acceptInvite(env: AuthEnv, email: string): Promise<void> {
  if (!env.DB) return;
  const e = normalizeEmail(email);
  const ws = workspaceID(env);
  const role: MemberRole = isAllowlisted(env, e) ? "owner" : "member";
  await env.DB.prepare(
    `INSERT INTO workspace_members (workspace_id, email, role, status, accepted_at)
     VALUES (?, ?, ?, 'active', datetime('now'))
     ON CONFLICT (workspace_id, email) DO UPDATE SET
       status = 'active',
       role = CASE WHEN excluded.role = 'owner' THEN 'owner' ELSE workspace_members.role END,
       accepted_at = COALESCE(workspace_members.accepted_at, datetime('now')),
       invite_token = NULL`,
  )
    .bind(ws, e, role)
    .run();
}

export async function revokeMember(
  env: AuthEnv,
  actorEmail: string,
  rawEmail: string,
): Promise<{ error?: string; status?: number }> {
  const email = normalizeEmail(rawEmail);
  await seedCaller(env, actorEmail);
  const members = await listMembers(env);
  if (!isOwnerEmail(env, actorEmail, members)) {
    return { error: "only owners can remove people", status: 403 };
  }
  if (email === normalizeEmail(actorEmail)) {
    return { error: "you cannot remove yourself", status: 400 };
  }
  if (isAllowlisted(env, email)) {
    return { error: "allowlisted owners cannot be removed here", status: 400 };
  }
  if (!env.DB) return { error: "D1 is required", status: 501 };
  await env.DB.prepare(
    `UPDATE workspace_members SET status = 'revoked', invite_token = NULL
     WHERE workspace_id = ? AND email = ?`,
  )
    .bind(workspaceID(env), email)
    .run();
  return {};
}
