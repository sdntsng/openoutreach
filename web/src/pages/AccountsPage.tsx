import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, asArray, type Account, type Capabilities, type DNSCheck } from "../api";
import { CONNECTORS, connectorEnabled } from "../connectors";
import { ConnectorCard } from "../ui";

export default function AccountsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [dns, setDns] = useState<Record<string, DNSCheck | string>>({});
  const connected = searchParams.get("connected") === "1";

  useEffect(() => {
    if (!connected) return;
    const next = new URLSearchParams(searchParams);
    next.delete("connected");
    setSearchParams(next, { replace: true });
  }, [connected, searchParams, setSearchParams]);

  async function reloadAccounts() {
    const data = await api.listAccounts();
    setAccounts(asArray(data, "accounts"));
  }

  useEffect(() => {
    Promise.all([api.listAccounts(), api.capabilities().catch(() => null)])
      .then(([data, c]) => {
        setAccounts(asArray(data, "accounts"));
        setCaps(c);
      })
      .catch((err: Error) => setError(err.message));
  }, []);

  const senders = CONNECTORS.filter((c) => c.kind === "send");

  async function toggleAccount(a: Account) {
    setBusy(true);
    setError(null);
    try {
      if (a.status === "paused") await api.resumeAccount(a.id);
      else await api.pauseAccount(a.id);
      await reloadAccounts();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Sending Accounts</h1>
        <Link to="/integrations?kind=send" className="row-actions">
          Manage connectors
        </Link>
      </div>
      <p className="muted">
        Connected mailboxes for campaigns. Add or rotate keys on{" "}
        <Link to="/integrations">Integrations</Link>. Warmup is a status badge only — it never sends
        via Tick.
      </p>
      {connected && <div className="panel">Account connected.</div>}
      {error && <div className="error">{error}</div>}

      <h2>Available senders</h2>
      <div className="connector-grid">
        {senders.map((c) => (
          <ConnectorCard
            key={c.id}
            connector={c}
            enabled={connectorEnabled(c, caps)}
            connected={accounts.some((a) =>
              (c.accountProviders || []).includes((a.provider || "").toLowerCase()),
            )}
            href={`/integrations?connect=${c.id}`}
          />
        ))}
      </div>

      <h2>Connected mailboxes</h2>
      <table>
        <thead>
          <tr>
            <th>Email</th>
            <th>Status</th>
            <th>Provider</th>
            <th>Reply</th>
            <th>Domain</th>
            <th>Warmup</th>
            <th>Daily limit</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {accounts.length === 0 ? (
            <tr>
              <td colSpan={8} className="muted">
                No mailboxes yet. Pick a sender above to connect.
              </td>
            </tr>
          ) : (
            accounts.map((a) => (
              <tr key={String(a.id)}>
                <td>{a.email}</td>
                <td>{a.status}</td>
                <td>{a.provider || "—"}</td>
                <td>{a.reply_mode || "—"}</td>
                <td>{a.domain_verification || "—"}</td>
                <td>{a.warmup_status && a.warmup_status !== "unset" ? a.warmup_status : "—"}</td>
                <td>{a.daily_limit ?? "—"}</td>
                <td className="row-actions">
                  <button
                    type="button"
                    className="secondary"
                    disabled={busy}
                    onClick={() => void toggleAccount(a)}
                  >
                    {a.status === "paused" ? "Resume" : "Pause"}
                  </button>
                  <button
                    type="button"
                    className="secondary"
                    disabled={busy}
                    onClick={() => {
                      const key = String(a.id);
                      setBusy(true);
                      api
                        .accountDNS(a.id)
                        .then((res) => setDns((prev) => ({ ...prev, [key]: res })))
                        .catch((err: Error) => setDns((prev) => ({ ...prev, [key]: err.message })))
                        .finally(() => setBusy(false));
                    }}
                  >
                    Check DNS
                  </button>
                  <button
                    type="button"
                    className="danger"
                    disabled={busy}
                    onClick={() => {
                      if (!window.confirm(`Remove ${a.email}? This does not send mail.`)) return;
                      setBusy(true);
                      api
                        .removeAccount(a.id)
                        .then(() => reloadAccounts())
                        .catch((err: Error) => setError(err.message))
                        .finally(() => setBusy(false));
                    }}
                  >
                    Remove
                  </button>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
      {Object.keys(dns).length > 0 && (
        <div className="panel" style={{ marginTop: "1rem" }}>
          <h3 style={{ marginTop: 0 }}>DNS (public MX / SPF / DMARC)</h3>
          {accounts.map((a) => {
            const row = dns[String(a.id)];
            if (!row) return null;
            if (typeof row === "string") {
              return (
                <p key={a.id} className="error">
                  {a.email}: {row}
                </p>
              );
            }
            return (
              <p key={a.id} className="muted">
                <strong>{row.email}</strong> — MX {row.mx ? "ok" : "missing"}, SPF{" "}
                {row.spf ? "ok" : "missing"}, DMARC {row.dmarc ? "ok" : "missing"}
              </p>
            );
          })}
        </div>
      )}
    </div>
  );
}
