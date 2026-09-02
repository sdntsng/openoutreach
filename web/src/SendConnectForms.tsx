import { useState, type FormEvent } from "react";
import { api } from "./api";
import { SecretField } from "./ui";

export function SMTPForm({
  ses,
  onDone,
  busy,
  setBusy,
  setError,
}: {
  ses?: boolean;
  onDone: () => void;
  busy: boolean;
  setBusy: (v: boolean) => void;
  setError: (v: string | null) => void;
}) {
  const [region, setRegion] = useState("us-east-1");
  const [form, setForm] = useState({
    email: "",
    smtp_host: ses ? "email-smtp.us-east-1.amazonaws.com" : "",
    smtp_port: ses ? "587" : "587",
    smtp_username: "",
    smtp_password: "",
    imap_host: "",
    imap_port: "993",
    imap_username: "",
    imap_password: "",
    daily_limit: "50",
  });

  function set<K extends keyof typeof form>(key: K, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.addSMTPAccount({
        email: form.email,
        daily_limit: Number(form.daily_limit) || 50,
        smtp_host: form.smtp_host,
        smtp_port: Number(form.smtp_port) || 587,
        smtp_username: form.smtp_username || form.email,
        smtp_password: form.smtp_password,
        smtp_tls_mode: "starttls",
        imap_host: form.imap_host || form.smtp_host,
        imap_port: Number(form.imap_port) || 993,
        imap_username: form.imap_username || form.smtp_username || form.email,
        imap_password: form.imap_password || form.smtp_password,
        imap_tls_mode: "tls",
      });
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={(e) => void onSubmit(e)}>
      {ses && (
        <label>
          AWS region
          <select
            value={region}
            onChange={(e) => {
              const next = e.target.value;
              setRegion(next);
              set("smtp_host", `email-smtp.${next}.amazonaws.com`);
            }}
          >
            {["us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1"].map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </label>
      )}
      <label>
        From email
        <input value={form.email} onChange={(e) => set("email", e.target.value)} required />
      </label>
      <label>
        SMTP host
        <input value={form.smtp_host} onChange={(e) => set("smtp_host", e.target.value)} required />
      </label>
      <label>
        SMTP port
        <select value={form.smtp_port} onChange={(e) => set("smtp_port", e.target.value)}>
          <option value="587">587 (STARTTLS)</option>
          <option value="465">465 (TLS)</option>
          <option value="25">25</option>
        </select>
      </label>
      <label>
        SMTP username
        <input value={form.smtp_username} onChange={(e) => set("smtp_username", e.target.value)} />
      </label>
      <SecretField
        label="SMTP password"
        value={form.smtp_password}
        onChange={(v) => set("smtp_password", v)}
        required
        placeholder="SMTP password"
      />
      {!ses && (
        <>
          <label>
            IMAP host
            <input value={form.imap_host} onChange={(e) => set("imap_host", e.target.value)} />
          </label>
          <label>
            IMAP port
            <select value={form.imap_port} onChange={(e) => set("imap_port", e.target.value)}>
              <option value="993">993 (TLS)</option>
              <option value="143">143</option>
            </select>
          </label>
          <SecretField
            label="IMAP password"
            value={form.imap_password}
            onChange={(v) => set("imap_password", v)}
            placeholder="Defaults to SMTP password"
          />
        </>
      )}
      <label>
        Daily limit
        <input value={form.daily_limit} onChange={(e) => set("daily_limit", e.target.value)} />
      </label>
      <button type="submit" disabled={busy}>
        {ses ? "Save SES SMTP account" : "Save SMTP account"}
      </button>
    </form>
  );
}

export function ResendForm({
  onDone,
  busy,
  setBusy,
  setError,
}: {
  onDone: () => void;
  busy: boolean;
  setBusy: (v: boolean) => void;
  setError: (v: string | null) => void;
}) {
  const [email, setEmail] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [limit, setLimit] = useState("50");

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.addResendAccount({ email, api_key: apiKey, daily_limit: Number(limit) || 50 });
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={(e) => void onSubmit(e)}>
      <label>
        From email
        <input value={email} onChange={(e) => setEmail(e.target.value)} required />
      </label>
      <SecretField label="API key" value={apiKey} onChange={setApiKey} required />
      <label>
        Daily limit
        <input value={limit} onChange={(e) => setLimit(e.target.value)} />
      </label>
      <button type="submit" disabled={busy}>
        Save Resend account
      </button>
    </form>
  );
}

export function CFEmailForm({
  onDone,
  busy,
  setBusy,
  setError,
}: {
  onDone: () => void;
  busy: boolean;
  setBusy: (v: boolean) => void;
  setError: (v: string | null) => void;
}) {
  const [email, setEmail] = useState("");
  const [accountId, setAccountId] = useState("");
  const [token, setToken] = useState("");
  const [limit, setLimit] = useState("50");

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.addCFEmailAccount({
        email,
        account_id: accountId,
        api_token: token,
        daily_limit: Number(limit) || 50,
      });
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="form-grid" onSubmit={(e) => void onSubmit(e)}>
      <label>
        From email
        <input value={email} onChange={(e) => setEmail(e.target.value)} required />
      </label>
      <label>
        Cloudflare account ID
        <input value={accountId} onChange={(e) => setAccountId(e.target.value)} required autoComplete="off" />
      </label>
      <SecretField label="API token" value={token} onChange={setToken} required />
      <label>
        Daily limit
        <input value={limit} onChange={(e) => setLimit(e.target.value)} />
      </label>
      <button type="submit" disabled={busy}>
        Save Cloudflare Email account
      </button>
    </form>
  );
}
