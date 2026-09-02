import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Capabilities } from "../api";
import { StatusBadge } from "../ui";

export default function SettingsPage() {
  const [workspaceId, setWorkspaceId] = useState<string>("…");
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

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

  return (
    <div>
      <h1>Settings</h1>
      {error && <div className="error">{error}</div>}
      {note && <div className="panel">{note}</div>}

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
          <input readOnly value={caps?.auth_mode || "—"} />
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
