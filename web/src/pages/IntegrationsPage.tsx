import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  api,
  asArray,
  type Account,
  type Capabilities,
  type IntegrationCredential,
} from "../api";
import {
  CONNECTORS,
  connectorConnected,
  connectorEnabled,
  type Connector,
  type ConnectorKind,
} from "../connectors";
import { CFEmailForm, ResendForm, SMTPForm } from "../SendConnectForms";
import { BrandMark, SecretField, StatusBadge } from "../ui";

const KIND_LABEL: Record<ConnectorKind | "all", string> = {
  all: "All",
  send: "Sending",
  leads: "Lead sources",
  events: "Events",
};

export default function IntegrationsPage() {
  const [params, setParams] = useSearchParams();
  const connect = params.get("connect") || "";
  const kind = (params.get("kind") as ConnectorKind | "all") || "all";
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [creds, setCreds] = useState<IntegrationCredential[]>([]);
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const connectedOAuth = params.get("connected") === "1";

  async function reload() {
    const [acc, ints, c] = await Promise.all([
      api.listAccounts(),
      api.listIntegrations().catch(() => ({ integrations: [] as IntegrationCredential[] })),
      api.capabilities().catch(() => null),
    ]);
    setAccounts(asArray(acc, "accounts"));
    setCreds(asArray(ints, "integrations"));
    setCaps(c);
  }

  useEffect(() => {
    reload().catch((err: Error) => setError(err.message));
  }, []);

  useEffect(() => {
    if (!connectedOAuth) return;
    const next = new URLSearchParams(params);
    next.delete("connected");
    setParams(next, { replace: true });
    setNote("Mailbox connected.");
  }, [connectedOAuth, params, setParams]);

  const selected = CONNECTORS.find((c) => c.id === connect) || null;
  const visible = useMemo(
    () => CONNECTORS.filter((c) => kind === "all" || c.kind === kind),
    [kind],
  );

  function open(id: string) {
    const next = new URLSearchParams(params);
    next.set("connect", id);
    setParams(next);
    setError(null);
    setNote(null);
  }

  function setKind(nextKind: ConnectorKind | "all") {
    const next = new URLSearchParams(params);
    if (nextKind === "all") next.delete("kind");
    else next.set("kind", nextKind);
    setParams(next);
  }

  return (
    <div>
      <div className="page-header">
        <h1>Integrations</h1>
      </div>
      <p className="muted">
        Route replies into tools you already use. Keys are encrypted; only the last four characters
        are shown. Sending Accounts lists connected mailboxes.
      </p>
      <section className="card featured-card">
        <div>
          <h2 style={{ margin: "0 0 0.35rem" }}>Outbound webhook</h2>
          <p className="muted" style={{ margin: 0 }}>
            New replies, bounces, and sends POST to your URL — Slack, Make, HubSpot workflows, or a
            Zap. Paste the URL under Events.
          </p>
        </div>
        <button type="button" onClick={() => open("outbound")}>
          Connect webhook
        </button>
      </section>
      {error && <div className="error">{error}</div>}
      {note && <div className="panel">{note}</div>}

      <div className="filters">
        {(["all", "send", "leads", "events"] as const).map((k) => (
          <button
            key={k}
            type="button"
            className={kind === k ? "active" : undefined}
            onClick={() => setKind(k)}
          >
            {KIND_LABEL[k]}
          </button>
        ))}
      </div>

      <div className="connector-grid">
        {visible.map((c) => {
          const enabled = connectorEnabled(c, caps);
          const on = connectorConnected(c, accounts, creds);
          return (
            <button
              key={c.id}
              type="button"
              className={`connector-card ${enabled ? "" : "is-off"} ${connect === c.id ? "is-active" : ""}`}
              onClick={() => open(c.id)}
            >
              <BrandMark connector={c} />
              <div className="connector-copy">
                <div className="connector-name">{c.name}</div>
                <p className="muted">{c.blurb}</p>
              </div>
              <StatusBadge
                ok={enabled && on}
                on={c.mode === "file" ? "Ready" : "Connected"}
                off={enabled ? "Not connected" : "Off"}
              />
            </button>
          );
        })}
      </div>

      {selected && (
        <section className="card stack" style={{ marginTop: "1.25rem" }}>
          <div className="row-actions" style={{ justifyContent: "space-between" }}>
            <div className="row-actions">
              <BrandMark connector={selected} />
              <h2 style={{ margin: 0 }}>{selected.name}</h2>
            </div>
            <button
              type="button"
              className="secondary"
              onClick={() => {
                const next = new URLSearchParams(params);
                next.delete("connect");
                setParams(next);
              }}
            >
              Close
            </button>
          </div>
          <p className="muted">{selected.blurb}</p>
          <ConnectorSetup
            connector={selected}
            caps={caps}
            accounts={accounts}
            creds={creds}
            busy={busy}
            setBusy={setBusy}
            setError={setError}
            setNote={setNote}
            onDone={() => void reload()}
          />
        </section>
      )}

      {creds.length > 0 && (
        <>
          <h2>Saved keys</h2>
          <p className="muted">Hints only. Delete and re-add to rotate.</p>
          <table>
            <thead>
              <tr>
                <th>Provider</th>
                <th>Name</th>
                <th>Key</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {creds.map((row) => (
                <tr key={row.id}>
                  <td>{row.provider}</td>
                  <td>{row.name}</td>
                  <td className="mono-hint">{row.secret_hint || "••••"}</td>
                  <td>
                    <StatusBadge ok={row.status !== "error"} on={row.status || "stored"} off="error" />
                  </td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="secondary"
                      disabled={busy}
                      onClick={() => {
                        setBusy(true);
                        api
                          .testIntegration(row.id)
                          .then((res) => setNote(res.ok ? `Test ok: ${res.detail}` : `Test failed: ${res.detail}`))
                          .catch((err: Error) => setError(err.message))
                          .finally(() => setBusy(false));
                      }}
                    >
                      Test
                    </button>
                    <button
                      type="button"
                      className="danger"
                      disabled={busy}
                      onClick={() => {
                        if (!window.confirm(`Remove ${row.provider}/${row.name}?`)) return;
                        setBusy(true);
                        api
                          .deleteIntegration(row.id)
                          .then(() => reload())
                          .catch((err: Error) => setError(err.message))
                          .finally(() => setBusy(false));
                      }}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      <section className="card stack" style={{ marginTop: "1.5rem" }}>
        <h2 style={{ margin: 0 }}>Build your own</h2>
        <p className="muted">
          Agents and scripts use the same API. Create stays draft — activate requires{" "}
          <code>confirm: true</code>.
        </p>
        <pre className="code">{`GET /api/v1/inbox?box=needs
GET /api/v1/campaigns
POST /api/v1/campaigns/{id}/activate  {"confirm":true}`}</pre>
        {caps?.mcp_endpoint ? (
          <p className="muted">
            MCP: <code>{caps.mcp_endpoint}</code> — bearer from Settings.
          </p>
        ) : (
          <p className="muted">
            MCP endpoint is on <Link to="/settings">Settings</Link>.
          </p>
        )}
      </section>
    </div>
  );
}

function ConnectorSetup({
  connector,
  caps,
  accounts,
  creds,
  busy,
  setBusy,
  setError,
  setNote,
  onDone,
}: {
  connector: Connector;
  caps: Capabilities | null;
  accounts: Account[];
  creds: IntegrationCredential[];
  busy: boolean;
  setBusy: (v: boolean) => void;
  setError: (v: string | null) => void;
  setNote: (v: string | null) => void;
  onDone: () => void;
}) {
  const enabled = connectorEnabled(connector, caps);
  if (!enabled) {
    return <p className="muted">This connector is off for this workspace (operator feature flag).</p>;
  }

  if (connector.mode === "file") {
    return (
      <p>
        Import CSV on <Link to="/leads">Leads</Link> or in a campaign. No API key.
      </p>
    );
  }

  if (connector.mode === "oauth") {
    const list = accounts.filter((a) =>
      (connector.accountProviders || []).includes((a.provider || "").toLowerCase()),
    );
    return (
      <div className="stack">
        {list.length > 0 && (
          <ul>
            {list.map((a) => (
              <li key={String(a.id)}>
                {a.email} — {a.status}
              </li>
            ))}
          </ul>
        )}
        <button
          type="button"
          disabled={busy}
          onClick={() => {
            setBusy(true);
            setError(null);
            const start = connector.id === "microsoft" ? api.startMicrosoftOAuth : api.startGoogleOAuth;
            start()
              .then((res) => {
                if (res.authorize_url) {
                  window.location.href = res.authorize_url;
                  return;
                }
                onDone();
              })
              .catch((err: Error) => setError(err.message))
              .finally(() => setBusy(false));
          }}
        >
          Connect {connector.name}
        </button>
      </div>
    );
  }

  if (connector.id === "smtp" || connector.id === "ses") {
    return (
      <SMTPForm
        ses={connector.id === "ses"}
        onDone={onDone}
        busy={busy}
        setBusy={setBusy}
        setError={setError}
      />
    );
  }
  if (connector.id === "resend") {
    return <ResendForm onDone={onDone} busy={busy} setBusy={setBusy} setError={setError} />;
  }
  if (connector.id === "cf_email") {
    return <CFEmailForm onDone={onDone} busy={busy} setBusy={setBusy} setError={setError} />;
  }

  return (
    <VaultForm
      connector={connector}
      creds={creds}
      caps={caps}
      busy={busy}
      setBusy={setBusy}
      setError={setError}
      setNote={setNote}
      onDone={onDone}
    />
  );
}

function VaultForm({
  connector,
  creds,
  caps,
  busy,
  setBusy,
  setError,
  setNote,
  onDone,
}: {
  connector: Connector;
  creds: IntegrationCredential[];
  caps: Capabilities | null;
  busy: boolean;
  setBusy: (v: boolean) => void;
  setError: (v: string | null) => void;
  setNote: (v: string | null) => void;
  onDone: () => void;
}) {
  const provider = connector.vaultProvider || connector.id;
  const existing = creds.find((r) => r.provider === provider);
  const [name, setName] = useState(existing?.name || "default");
  const [secret, setSecret] = useState("");
  const ingest =
    connector.mode === "webhook"
      ? `${(caps?.public_base_url || "").replace(/\/$/, "")}/api/v1/integrations/${provider === "webhook" ? "generic" : provider}/ingest?name=${encodeURIComponent(name || "default")}`
      : "";

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.putIntegration({ provider, name, secret });
      if (connector.mode === "webhook") {
        await api.putWebhookEndpoint({
          provider: provider === "webhook" ? "generic" : provider,
          name,
          hmac_secret: secret,
        });
      }
      setSecret("");
      setNote(`${connector.name} saved. Key is encrypted.`);
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={(e) => void onSubmit(e)}>
      {existing && (
        <p className="muted">
          Stored as <code>{existing.name}</code> · {existing.secret_hint || "••••"}
        </p>
      )}
      <label>
        Credential name
        <input value={name} onChange={(e) => setName(e.target.value)} required />
      </label>
      <SecretField
        label={connector.id === "outbound" ? "Webhook URL" : "API key / HMAC secret"}
        value={secret}
        onChange={setSecret}
        required
        placeholder={connector.id === "outbound" ? "https://hooks.example.com/…" : undefined}
      />
      {ingest && (
        <label>
          Ingest URL
          <input readOnly value={ingest || "Set PUBLIC_BASE_URL to show the full URL"} />
        </label>
      )}
      {ingest && (
        <button
          type="button"
          className="secondary"
          onClick={() => {
            void navigator.clipboard.writeText(ingest).then(
              () => setNote("Ingest URL copied."),
              () => setNote("Copy failed — select the field."),
            );
          }}
        >
          Copy ingest URL
        </button>
      )}
      <button type="submit" disabled={busy || !secret}>
        Save key
      </button>
    </form>
  );
}
