import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign, type Lead, type VerifyResult } from "../api";
import { LeadImport } from "../LeadImport";
import { StatusBadge } from "../ui";

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
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [verifyText, setVerifyText] = useState("");
  const [verifyRows, setVerifyRows] = useState<VerifyResult[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function load(query?: string) {
    setError(null);
    try {
      const [data, camps] = await Promise.all([
        api.listLeads(query),
        api.listCampaigns().catch(() => ({ campaigns: [] as Campaign[] })),
      ]);
      setLeads(asArray(data, "leads"));
      setCampaigns(asArray(camps, "campaigns"));
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
      <LeadImport campaigns={campaigns} onImported={() => void load(q.trim() || undefined)} />
      <h2>Directory</h2>
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
                No leads yet. Import a CSV or a connector above.
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

      <p className="muted">
        Org-wide blocks live on the <Link to="/suppressions">suppress list</Link>.
      </p>

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
                <td>
                  <StatusBadge ok={r.ok} on="Valid" off="Invalid" />
                </td>
                <td>{r.reason || (r.mx ? "mx" : "—")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
