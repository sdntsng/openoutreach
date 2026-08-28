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
  const [showResend, setShowResend] = useState(false);
  const [showCF, setShowCF] = useState(false);
  const [resend, setResend] = useState({ email: "", api_key: "", daily_limit: "50" });
  const [cfEmail, setCfEmail] = useState({
    email: "",
    api_token: "",
    account_id: "",
    daily_limit: "50",
  });
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

  async function connectGoogle() {
    setBusy(true);
    setError(null);
    try {
      const res = await api.startGoogleOAuth();
      if (res.authorize_url) {
        window.location.href = res.authorize_url;
        return;
      }
      await reloadAccounts();
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
      await reloadAccounts();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onAddResend(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.addResendAccount({
        email: resend.email,
        api_key: resend.api_key,
        daily_limit: Number(resend.daily_limit) || 50,
      });
      setShowResend(false);
      await reloadAccounts();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onAddCFEmail(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.addCFEmailAccount({
        email: cfEmail.email,
        api_token: cfEmail.api_token,
        account_id: cfEmail.account_id,
        daily_limit: Number(cfEmail.daily_limit) || 50,
      });
      setShowCF(false);
      await reloadAccounts();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function toggleAccount(a: Account) {
    setBusy(true);
    setError(null);
    try {
      if (a.status === "paused") {
        await api.resumeAccount(a.id);
      } else {
        await api.pauseAccount(a.id);
      }
      await reloadAccounts();
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
          {caps?.sending?.resend && (
            <button type="button" disabled={busy} onClick={() => setShowResend((v) => !v)}>
              Add Resend
            </button>
          )}
          {caps?.sending?.cf_email && (
            <button type="button" disabled={busy} onClick={() => setShowCF((v) => !v)}>
              Add Cloudflare Email
            </button>
          )}
        </div>
      </div>
      <p className="muted">
        Connect Gmail, Microsoft 365, SMTP/IMAP, or a send-only API mailer. Cloudflare Email uses
        Email Routing for replies. Domain verification for Resend/SES is DNS at the provider. Warmup
        is a status badge only — it never sends via Tick.
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
      {showResend && (
        <form className="panel form-grid" onSubmit={(e) => void onAddResend(e)} style={{ marginBottom: "1rem" }}>
          <h3 style={{ marginTop: 0 }}>Resend (send-only)</h3>
          <p className="muted">API mailer. Weak for cold-inbox reputation; configure bounce webhook. Not Instantly.</p>
          <label>
            From email
            <input value={resend.email} onChange={(e) => setResend({ ...resend, email: e.target.value })} required />
          </label>
          <label>
            API key
            <input
              type="password"
              value={resend.api_key}
              onChange={(e) => setResend({ ...resend, api_key: e.target.value })}
              required
              autoComplete="off"
            />
          </label>
          <label>
            Daily limit
            <input value={resend.daily_limit} onChange={(e) => setResend({ ...resend, daily_limit: e.target.value })} />
          </label>
          <button type="submit" disabled={busy}>
            Save Resend account
          </button>
        </form>
      )}
      {showCF && (
        <form className="panel form-grid" onSubmit={(e) => void onAddCFEmail(e)} style={{ marginBottom: "1rem" }}>
          <h3 style={{ marginTop: 0 }}>Cloudflare Email Sending</h3>
          <p className="muted">
            Transactional Email Service (not Instantly). Requires Workers Paid to send to arbitrary
            recipients. Route inbound mail for this domain to this Worker. SMTP alternative:{" "}
            <code>smtp.mx.cloudflare.net:465</code> user <code>api_token</code> — still no IMAP;
            replies stay on Email Routing.
          </p>
          <label>
            From email
            <input
              value={cfEmail.email}
              onChange={(e) => setCfEmail({ ...cfEmail, email: e.target.value })}
              required
            />
          </label>
          <label>
            Cloudflare account ID
            <input
              value={cfEmail.account_id}
              onChange={(e) => setCfEmail({ ...cfEmail, account_id: e.target.value })}
              required
              autoComplete="off"
            />
          </label>
          <label>
            API token
            <input
              type="password"
              value={cfEmail.api_token}
              onChange={(e) => setCfEmail({ ...cfEmail, api_token: e.target.value })}
              required
              autoComplete="off"
            />
          </label>
          <label>
            Daily limit
            <input
              value={cfEmail.daily_limit}
              onChange={(e) => setCfEmail({ ...cfEmail, daily_limit: e.target.value })}
            />
          </label>
          <button type="submit" disabled={busy}>
            Save Cloudflare Email account
          </button>
        </form>
      )}
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
                No accounts connected. Connect Gmail or Microsoft, or add SMTP/IMAP. Cloudflare Email
                appears when <code>FEATURE_CF_EMAIL=1</code>.
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
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}
