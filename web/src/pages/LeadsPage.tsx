import { useEffect, useState, type FormEvent } from "react";
import { api, asArray, type Lead, type Suppression, type VerifyResult } from "../api";

function downloadCSV(filename: string, csv: string) {
  const blob = new Blob([csv], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export default function LeadsPage() {
  const [q, setQ] = useState("");
  const [leads, setLeads] = useState<Lead[]>([]);
  const [suppressions, setSuppressions] = useState<Suppression[]>([]);
  const [block, setBlock] = useState("");
  const [verifyText, setVerifyText] = useState("");
  const [verifyRows, setVerifyRows] = useState<VerifyResult[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function load(query?: string) {
    setError(null);
    try {
      const [data, sup] = await Promise.all([
        api.listLeads(query),
        api.listSuppressions().catch(() => ({ suppressions: [] as Suppression[] })),
      ]);
      setLeads(asArray(data, "leads"));
      setSuppressions(asArray(sup, "suppressions"));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function onSearch(e: FormEvent) {
    e.preventDefault();
    void load(q.trim() || undefined);
  }

  async function blacklist(id: string | number, email: string) {
    if (!window.confirm(`Blacklist ${email}? Pending sends will be cancelled.`)) return;
    setBusy(true);
    try {
      await api.blacklistLead(id);
      await load(q.trim() || undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onSuppress(e: FormEvent) {
    e.preventDefault();
    const value = block.trim().toLowerCase();
    if (!value) return;
    setBusy(true);
    setError(null);
    try {
      if (value.includes("@")) {
        await api.addSuppression({ email: value });
      } else {
        await api.addSuppression({ domain: value });
      }
      setBlock("");
      await load(q.trim() || undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onVerify(e: FormEvent) {
    e.preventDefault();
    const emails = verifyText
      .split(/[\s,;]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (emails.length === 0) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api.verifyLeads({ emails });
      setVerifyRows(res.results || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Leads</h1>
        <button
          type="button"
          className="secondary"
          disabled={busy}
          onClick={() => {
            void api
              .exportLeads(q.trim() || undefined)
              .then((res) => downloadCSV("leads.csv", res.csv))
              .catch((err: Error) => setError(err.message));
          }}
        >
          Export CSV
        </button>
      </div>
      <form className="row-actions" onSubmit={onSearch} style={{ marginBottom: "1rem" }}>
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search email, name, company…"
          style={{ minWidth: 260 }}
        />
        <button type="submit">Search</button>
      </form>
      {error && <div className="error">{error}</div>}
      <table>
        <thead>
          <tr>
            <th>Email</th>
            <th>Name</th>
            <th>Company</th>
            <th>Status</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {leads.length === 0 ? (
            <tr>
              <td colSpan={5} className="muted">
                No leads. Import CSV on a campaign, or use Sheets / Apollo / Clay ingest.
              </td>
            </tr>
          ) : (
            leads.map((l) => (
              <tr key={String(l.id)}>
                <td>{l.email}</td>
                <td>
                  {[l.first_name, l.last_name].filter(Boolean).join(" ") || "—"}
                </td>
                <td>{l.company || "—"}</td>
                <td>{l.global_status || "active"}</td>
                <td>
                  {l.global_status !== "blacklisted" && (
                    <button
                      type="button"
                      className="danger"
                      disabled={busy}
                      onClick={() => void blacklist(l.id, l.email)}
                    >
                      Blacklist
                    </button>
                  )}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>

      <h2>Suppressions</h2>
      <p className="muted">Emails and domains stay blocked on future imports, even if the lead is not in the list yet.</p>
      <form className="row-actions" onSubmit={onSuppress} style={{ marginBottom: "0.75rem" }}>
        <input
          value={block}
          onChange={(e) => setBlock(e.target.value)}
          placeholder="email@domain.com or example.com"
          style={{ minWidth: 260 }}
        />
        <button type="submit" disabled={busy || !block.trim()}>
          Add
        </button>
      </form>
      <table>
        <thead>
          <tr>
            <th>Kind</th>
            <th>Value</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {suppressions.length === 0 ? (
            <tr>
              <td colSpan={3} className="muted">
                No suppressions yet.
              </td>
            </tr>
          ) : (
            suppressions.map((s) => (
              <tr key={s.id}>
                <td>{s.kind}</td>
                <td>{s.value}</td>
                <td>
                  <button
                    type="button"
                    className="secondary"
                    disabled={busy}
                    onClick={() => {
                      void api
                        .deleteSuppression(s.id)
                        .then(() => load(q.trim() || undefined))
                        .catch((err: Error) => setError(err.message));
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

      <h2>Verify emails</h2>
      <p className="muted">Syntax, disposable-domain list, and public MX. No API key.</p>
      <form className="card stack" onSubmit={onVerify}>
        <textarea
          rows={4}
          value={verifyText}
          onChange={(e) => setVerifyText(e.target.value)}
          placeholder="one email per line"
        />
        <button type="submit" disabled={busy || !verifyText.trim()}>
          Verify
        </button>
      </form>
      {verifyRows.length > 0 && (
        <table>
          <thead>
            <tr>
              <th>Email</th>
              <th>OK</th>
              <th>Reason</th>
            </tr>
          </thead>
          <tbody>
            {verifyRows.map((r) => (
              <tr key={r.email}>
                <td>{r.email}</td>
                <td>{r.ok ? "yes" : "no"}</td>
                <td>{r.reason || (r.mx ? "mx" : "—")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
