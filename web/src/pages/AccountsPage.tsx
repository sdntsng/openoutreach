import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, asArray, type Account } from "../api";

export default function AccountsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const connected = searchParams.get("connected") === "1";

  useEffect(() => {
    if (!connected) return;
    const next = new URLSearchParams(searchParams);
    next.delete("connected");
    setSearchParams(next, { replace: true });
  }, [connected, searchParams, setSearchParams]);

  useEffect(() => {
    api
      .listAccounts()
      .then((data) => setAccounts(asArray(data, "accounts")))
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
      // Mock mode returns connected status without redirect.
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
        <button type="button" disabled={busy} onClick={() => void connectGoogle()}>
          Connect Google
        </button>
      </div>
      <p className="muted">
        Connect a Gmail / Google Workspace mailbox. OAuth starts via{" "}
        <code>GET /api/v1/accounts/google/oauth/start</code>.
      </p>
      {connected && (
        <div className="panel" style={{ marginBottom: "1rem" }}>
          Google account connected.
        </div>
      )}
      {error && <div className="error">{error}</div>}
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
