import { useEffect, useState, type FormEvent } from "react";
import { api, asArray, type Lead } from "../api";

export default function LeadsPage() {
  const [q, setQ] = useState("");
  const [leads, setLeads] = useState<Lead[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function load(query?: string) {
    setError(null);
    try {
      const data = await api.listLeads(query);
      setLeads(asArray(data, "leads"));
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

  return (
    <div>
      <h1>Leads</h1>
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
    </div>
  );
}
