import { FormEvent, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, asArray, type Account, type Capabilities } from "../api";

export default function AccountsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [caps, setCaps] = useState<Capabilities | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [showSMTP, setShowSMTP] = useState(false);
  const [smtp, setSmtp] = useState({
    email: "",
    smtp_host: "",
    smtp_port: "587",
    smtp_username: "",
    smtp_password: "",
    imap_host: "",
    imap_port: "993",
    imap_username: "",
    imap_password: "",
    daily_limit: "50",
  });
  const connected = searchParams.get("connected") === "1";

  useEffect(() => {
    if (!connected) return;
    const next = new URLSearchParams(searchParams);
    next.delete("connected");
    setSearchParams(next, { replace: true });
  }, [connected, searchParams, setSearchParams]);

  useEffect(() => {
    Promise.all([api.listAccounts(), api.capabilities().catch(() => null)])
      .then(([data, c]) => {
        setAccounts(asArray(data, "accounts"));
        setCaps(c);
      })
      .catch((err: Error) => setError(err.message));
  }, []);

  async function connectGoogle() {
    setBusy(true);
    setError(null);
    try {
      const res = await api.startGoogleOAuth();
      if (res.authorize_url) {
        window.location.href = res.authorize_url;
        return;
      }
      const data = await api.listAccounts();
      setAccounts(asArray(data, "accounts"));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function connectMicrosoft() {
    setBusy(true);
    setError(null);
    try {
      const res = await api.startMicrosoftOAuth();
      if (res.authorize_url) {
        window.location.href = res.authorize_url;
        return;
      }
      setError("Microsoft OAuth did not return authorize_url");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onAddSMTP(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.addSMTPAccount({
        email: smtp.email,
        daily_limit: Number(smtp.daily_limit) || 50,
        smtp_host: smtp.smtp_host,
        smtp_port: Number(smtp.smtp_port) || 587,
        smtp_username: smtp.smtp_username || smtp.email,
        smtp_password: smtp.smtp_password,
        smtp_tls_mode: "starttls",
        imap_host: smtp.imap_host || smtp.smtp_host,
        imap_port: Number(smtp.imap_port) || 993,
        imap_username: smtp.imap_username || smtp.smtp_username || smtp.email,
        imap_password: smtp.imap_password || smtp.smtp_password,
        imap_tls_mode: "tls",
      });
      setShowSMTP(false);
      const data = await api.listAccounts();
      setAccounts(asArray(data, "accounts"));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div className="row-actions" style={{ justifyContent: "space-between", marginBottom: "1rem" }}>
        <h1 style={{ margin: 0 }}>Sending Accounts</h1>
        <div className="row-actions">
          {(!caps || caps.sending?.gmail) && (
            <button type="button" disabled={busy} onClick={() => void connectGoogle()}>
              Connect Google
            </button>
          )}
          {caps?.sending?.microsoft && (
            <button type="button" disabled={busy} onClick={() => void connectMicrosoft()}>
              Connect Microsoft
            </button>
          )}
          {(!caps || caps.sending?.smtp_imap) && (
            <button type="button" disabled={busy} onClick={() => setShowSMTP((v) => !v)}>
              Add SMTP/IMAP
            </button>
          )}
        </div>
      </div>
      <p className="muted">
        Connect Gmail, Microsoft 365, or SMTP/IMAP mailboxes. OAuth callbacks never expose tokens to the browser.
      </p>
      {connected && (
        <div className="panel" style={{ marginBottom: "1rem" }}>
          Account connected.
        </div>
      )}
      {error && <div className="error">{error}</div>}
      {showSMTP && (
        <form className="panel form-grid" onSubmit={(e) => void onAddSMTP(e)} style={{ marginBottom: "1rem" }}>
          <h3 style={{ marginTop: 0 }}>SMTP / IMAP</h3>
          {(
            [
              ["email", "Email"],
              ["smtp_host", "SMTP host"],
              ["smtp_port", "SMTP port"],
              ["smtp_username", "SMTP username"],
              ["smtp_password", "SMTP password"],
              ["imap_host", "IMAP host"],
              ["imap_port", "IMAP port"],
              ["imap_username", "IMAP username"],
              ["imap_password", "IMAP password"],
              ["daily_limit", "Daily limit"],
            ] as const
          ).map(([key, label]) => (
            <label key={key}>
              {label}
              <input
                type={key.includes("password") ? "password" : "text"}
                value={smtp[key]}
                onChange={(e) => setSmtp({ ...smtp, [key]: e.target.value })}
                required={key === "email" || key === "smtp_host" || key === "smtp_password"}
              />
            </label>
          ))}
          <button type="submit" disabled={busy}>
            Save SMTP account
          </button>
        </form>
      )}
      <table>
        <thead>
          <tr>
            <th>Email</th>
            <th>Status</th>
            <th>Provider</th>
            <th>Daily limit</th>
          </tr>
        </thead>
        <tbody>
          {accounts.length === 0 ? (
            <tr>
              <td colSpan={4} className="muted">
                No accounts connected.
              </td>
            </tr>
          ) : (
            accounts.map((a) => (
              <tr key={String(a.id)}>
                <td>{a.email}</td>
                <td>{a.status}</td>
                <td>{a.provider || "—"}</td>
                <td>{a.daily_limit ?? "—"}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}
