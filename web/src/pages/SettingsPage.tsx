import { FormEvent, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Capabilities, type IntegrationCredential } from "../api";

export default function SettingsPage() {
  const [workspaceId, setWorkspaceId] = useState<string>("…");
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [integrations, setIntegrations] = useState<IntegrationCredential[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [provider, setProvider] = useState("apollo");
  const [name, setName] = useState("default");
  const [secret, setSecret] = useState("");
  const [outboundURL, setOutboundURL] = useState("");
  const [busy, setBusy] = useState(false);

  const integrationChoices = (
    [
      ["apollo", "apollo"],
      ["clay", "clay"],
      ["webhook", "webhook"],
      ["outbound", "outbound"],
      ["sheets", "sheets"],
      ["hunter", "hunter"],
      ["warmup", "warmup"],
      ["secret", "secret"],
    ] as const
  ).filter(([key]) => {
    if (!caps?.integrations) return true;
    if (!(key in caps.integrations)) return true;
    return Boolean(caps.integrations[key]);
  });

  async function reload() {
    const [w, c] = await Promise.all([api.workspace(), api.capabilities()]);
    setWorkspaceId(w.workspace_id || "default");
    setCaps(c);
    try {
      const ints = await api.listIntegrations();
      setIntegrations(asArray(ints, "integrations"));
    } catch {
      setIntegrations([]);
    }
  }

  useEffect(() => {
    reload().catch((err: Error) => {
      setWorkspaceId("default");
      setError(err.message);
    });
  }, []);

  useEffect(() => {
    if (integrationChoices.length === 0) return;
    if (!integrationChoices.some(([key]) => key === provider)) {
      setProvider(integrationChoices[0][0]);
    }
  }, [caps, provider]);

  async function onSave(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      await api.putIntegration({ provider, name, secret });
      setSecret("");
      setNote("Credential saved (masked).");
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onTest(id: number) {
    setBusy(true);
    setError(null);
    try {
      const res = await api.testIntegration(id);
      setNote(res.ok ? `Test ok: ${res.detail}` : `Test failed: ${res.detail}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onDelete(id: number) {
    setBusy(true);
    try {
      await api.deleteIntegration(id);
      await reload();
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

      <h2>Workspace</h2>
      <div className="panel form-grid">
        <label>
          Workspace ID
          <input readOnly value={workspaceId} />
        </label>
        <p className="muted">
          Hosted requests resolve workspace from server config (
          <code>OPENOUTREACH_WORKSPACE_ID</code>), not from email domain.
        </p>
      </div>

      <h2>Sending</h2>
      <div className="panel">
        <p className="muted">
          Operator-enabled mailbox providers. Connect mailboxes on{" "}
          <Link to="/accounts">Sending Accounts</Link>.
        </p>
        <ul>
          {caps &&
            Object.entries(caps.sending || {}).map(([k, v]) => (
              <li key={k}>
                <code>{k}</code>: {v ? "enabled" : "disabled"}
              </li>
            ))}
        </ul>
      </div>

      <h2>Outbound webhook</h2>
      <div className="panel form-grid">
        <p className="muted">
          Paste a Slack / Make / HubSpot incoming URL. After each tick, new sent / reply / bounce
          events are POSTed as JSON. Failures never block sending.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setBusy(true);
            setError(null);
            setNote(null);
            api
              .putIntegration({ provider: "outbound", name: "default", secret: outboundURL })
              .then(() => {
                setOutboundURL("");
                setNote("Outbound webhook saved.");
                return reload();
              })
              .catch((err: Error) => setError(err.message))
              .finally(() => setBusy(false));
          }}
          className="form-grid"
        >
          <label>
            Webhook URL
            <input
              value={outboundURL}
              onChange={(e) => setOutboundURL(e.target.value)}
              placeholder="https://hooks.example.com/…"
              required
            />
          </label>
          <button type="submit" disabled={busy}>
            Save webhook
          </button>
        </form>
      </div>

      <h2>Integrations</h2>
      <div className="panel form-grid">
        <p className="muted">
          Workspace API keys (Apollo, Clay HMAC, etc.). Secrets are encrypted and never returned in full.
        </p>
        <form onSubmit={(e) => void onSave(e)} className="form-grid">
          <label>
            Provider
            <select value={provider} onChange={(e) => setProvider(e.target.value)}>
              {integrationChoices.map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Secret / API key
            <input
              type="password"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              required
              autoComplete="off"
            />
          </label>
          <button type="submit" disabled={busy}>
            Save credential
          </button>
        </form>
        <table>
          <thead>
            <tr>
              <th>Provider</th>
              <th>Name</th>
              <th>Hint</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {integrations.length === 0 ? (
              <tr>
                <td colSpan={5} className="muted">
                  No integration credentials yet.
                </td>
              </tr>
            ) : (
              integrations.map((row) => (
                <tr key={row.id}>
                  <td>{row.provider}</td>
                  <td>{row.name}</td>
                  <td>{row.secret_hint || "****"}</td>
                  <td>{row.status}</td>
                  <td className="row-actions">
                    <button type="button" disabled={busy} onClick={() => void onTest(row.id)}>
                      Test
                    </button>
                    <button type="button" disabled={busy} onClick={() => void onDelete(row.id)}>
                      Delete
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
        {caps && (
          <ul className="muted">
            {Object.entries(caps.integrations || {}).map(([k, v]) => (
              <li key={k}>
                feature <code>{k}</code>: {v ? "on" : "off"}
              </li>
            ))}
          </ul>
        )}
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
        <label>
          Bearer configured
          <input readOnly value={caps?.mcp_configured ? "yes" : "no"} />
        </label>
        <p className="muted">
          Agents use <code>Authorization: Bearer $MCP_BEARER_TOKEN</code>. Tokens are never shown here.
        </p>
      </div>

      <h2>Auth</h2>
      <div className="panel form-grid">
        <label>
          AUTH_MODE
          <input readOnly value={caps?.auth_mode || "—"} />
        </label>
        <p className="muted">
          Encryption vault ready: {caps?.encryption_ready ? "yes" : "no"}. Google OAuth:{" "}
          {caps?.google_oauth_ready ? "yes" : "no"}. Microsoft OAuth:{" "}
          {caps?.microsoft_oauth_ready ? "yes" : "no"}.
        </p>
      </div>
    </div>
  );
}
