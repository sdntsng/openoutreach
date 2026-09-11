import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api, type Capabilities, type TeamMember } from "../api";
import { useAuth } from "../auth-client";
import { StatusBadge } from "../ui";

export default function SettingsPage() {
  const auth = useAuth();
  const [workspaceId, setWorkspaceId] = useState<string>("…");
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteURL, setInviteURL] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const hosted = auth.mode === "hosted";

  async function reloadTeam() {
    if (auth.mode !== "hosted") return;
    const data = await api.listTeam();
    setMembers(data.members || []);
  }

  useEffect(() => {
    Promise.all([api.workspace(), api.capabilities()])
      .then(([w, c]) => {
        setWorkspaceId(w.workspace_id || "default");
        setCaps(c);
      })
      .catch((err: Error) => {
        setWorkspaceId("default");
        setError(err.message);
      });
  }, []);

  useEffect(() => {
    if (!hosted) return;
    reloadTeam().catch((err: Error) => setError(err.message));
  }, [hosted]);

  async function onInvite(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setNote(null);
    setInviteURL(null);
    try {
      const member = await api.inviteTeam(inviteEmail);
      setInviteEmail("");
      setInviteURL(member.invite_url || null);
      setNote(`Invite created for ${member.email}. Copy the link — we do not send mail.`);
      await reloadTeam();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <h1>Settings</h1>
      {error && <div className="error">{error}</div>}
      {note && <div className="panel">{note}</div>}

      <h2 id="team">People</h2>
      <div className="panel stack">
        {hosted ? (
          <>
            <p className="muted" style={{ marginTop: 0 }}>
              Invite someone to this project. They set a password on the link and share this
              workspace — campaigns, mailbox, leads. Workspace id still comes from config, not
              email domain.
            </p>
            <form className="row-actions" onSubmit={(e) => void onInvite(e)}>
              <input
                type="email"
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                placeholder="teammate@company.com"
                required
              />
              <button type="submit" disabled={busy || !inviteEmail.trim()}>
                {busy ? "Inviting…" : "Invite"}
              </button>
            </form>
            {inviteURL ? (
              <div className="row-actions">
                <input readOnly value={inviteURL} />
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    void navigator.clipboard.writeText(inviteURL).then(
                      () => setNote("Invite link copied."),
                      () => setNote("Copy failed — select the link and copy manually."),
                    );
                  }}
                >
                  Copy link
                </button>
              </div>
            ) : null}
            {members.length > 0 ? (
              <table>
                <thead>
                  <tr>
                    <th>Email</th>
                    <th>Role</th>
                    <th>Status</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {members.map((m) => (
                    <tr key={m.email}>
                      <td>{m.email}</td>
                      <td>{m.role}</td>
                      <td>
                        <StatusBadge
                          ok={m.status === "active"}
                          on="Active"
                          off={m.status === "invited" ? "Invited" : "Revoked"}
                        />
                      </td>
                      <td className="row-actions">
                        {m.role !== "owner" && m.status !== "revoked" ? (
                          <button
                            type="button"
                            className="secondary"
                            disabled={busy}
                            onClick={() => {
                              setBusy(true);
                              api
                                .revokeTeam(m.email)
                                .then(() => reloadTeam())
                                .catch((err: Error) => setError(err.message))
                                .finally(() => setBusy(false));
                            }}
                          >
                            Remove
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="muted">You are the first person here. Invite with an email.</p>
            )}
          </>
        ) : (
          <p className="muted">
            This instance uses Cloudflare Access (email OTP / IdP). Access has no password
            database. Set <code>AUTH_MODE=hosted</code> on the Worker, turn off the Access
            application for this hostname, then invite people here with a password link.
          </p>
        )}
      </div>

      <h2>Workspace</h2>
      <div className="panel form-grid">
        <label>
          Workspace ID
          <input readOnly value={workspaceId} />
        </label>
        <p className="muted">
          Hosted requests resolve workspace from <code>OPENOUTREACH_WORKSPACE_ID</code>, not email
          domain.
        </p>
      </div>

      <h2>Integrations</h2>
      <div className="panel">
        <p className="muted">
          API keys, OAuth, SMTP, and ingest URLs live on the Integrations page — not here.
        </p>
        <Link to="/integrations">Open Integrations</Link>
      </div>

      <h2>Sending providers</h2>
      <div className="panel">
        <p className="muted">
          Operator-enabled mailbox types. Connect them on{" "}
          <Link to="/integrations?kind=send">Integrations</Link>.
        </p>
        <ul className="flag-list">
          {caps &&
            Object.entries(caps.sending || {}).map(([k, v]) => (
              <li key={k}>
                <code>{k}</code> <StatusBadge ok={v} on="On" off="Off" />
              </li>
            ))}
        </ul>
      </div>

      <h2>MCP</h2>
      <div className="panel form-grid">
        <label>
          Endpoint
          <input readOnly value={caps?.mcp_endpoint || "—"} />
        </label>
        <div className="row-actions">
          <button
            type="button"
            className="secondary"
            disabled={!caps?.mcp_endpoint}
            onClick={() => {
              const url = caps?.mcp_endpoint || "";
              if (!url) return;
              void navigator.clipboard.writeText(url).then(
                () => setNote("MCP endpoint copied."),
                () => setNote("Copy failed — select the endpoint field and copy manually."),
              );
            }}
          >
            Copy
          </button>
        </div>
        <div className="row-actions">
          <span>Bearer configured</span>
          <StatusBadge ok={Boolean(caps?.mcp_configured)} on="Yes" off="No" />
        </div>
        <p className="muted">
          Agents use <code>Authorization: Bearer $MCP_BEARER_TOKEN</code>. Tokens are never shown
          here.
        </p>
      </div>

      <h2>Auth</h2>
      <div className="panel form-grid">
        <label>
          AUTH_MODE
          <input readOnly value={caps?.auth_mode || auth.mode || "—"} />
        </label>
        <div className="flag-list">
          <div>
            Encryption vault <StatusBadge ok={Boolean(caps?.encryption_ready)} on="Ready" off="Not ready" />
          </div>
          <div>
            Google OAuth <StatusBadge ok={Boolean(caps?.google_oauth_ready)} on="Ready" off="Not ready" />
          </div>
          <div>
            Microsoft OAuth{" "}
            <StatusBadge ok={Boolean(caps?.microsoft_oauth_ready)} on="Ready" off="Not ready" />
          </div>
        </div>
      </div>
    </div>
  );
}
