import { useEffect, useState, type FormEvent } from "react";
import { api, asArray, type Suppression } from "../api";
import { FileDrop, PageIntro } from "../ui";

export default function SuppressionsPage() {
  const [people, setPeople] = useState<Suppression[]>([]);
  const [companies, setCompanies] = useState<Suppression[]>([]);
  const [email, setEmail] = useState("");
  const [domains, setDomains] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function load() {
    const data = await api.listSuppressions();
    const all = asArray(data, "suppressions");
    setPeople(all.filter((s) => s.kind === "email"));
    setCompanies(all.filter((s) => s.kind === "domain"));
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, []);

  async function addEmails(raw: string) {
    const value = raw.trim();
    if (!value) return;
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      if (value.includes(",") || value.includes("\n") || value.includes("@") === false) {
        const res = (await api.addSuppression({ csv: value })) as { added?: number; skipped?: number };
        setNote(`Added ${res.added ?? 0} people` + (res.skipped ? ` · skipped ${res.skipped}` : ""));
      } else {
        await api.addSuppression({ email: value.toLowerCase() });
      }
      setEmail("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function addDomains(raw: string) {
    const value = raw.trim();
    if (!value) return;
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      if (value.includes("\n") || value.includes(",") || value.includes(" ")) {
        const res = (await api.addSuppression({ kind: "domain", csv: value })) as { added?: number };
        setNote(`Added ${res.added ?? 0} companies`);
      } else {
        await api.addSuppression({ domain: value.toLowerCase() });
      }
      setDomains("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageIntro title="Suppress list">
        Workspace-wide — one list, applied on every import and send. Matched by email or domain.
      </PageIntro>
      {error && <div className="error">{error}</div>}
      {note && <div className="panel">{note}</div>}

      <section className="card stack" style={{ marginBottom: "1rem" }}>
        <h2 style={{ margin: 0 }}>People</h2>
        <p className="muted">Specific people we should never email — matched by address.</p>
        <form
          className="row-actions"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            void addEmails(email);
          }}
        >
          <input
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="name@acme.com or paste a list"
            style={{ minWidth: 260, flex: 1 }}
          />
          <button type="submit" disabled={busy || !email.trim()}>
            + Add
          </button>
        </form>
        <FileDrop
          label="Drop CSV or click to upload"
          hint="Export from your CRM as-is — we'll take the email column."
          onText={(text) => void addEmails(text)}
        />
        <SuppressionTable
          rows={people}
          empty="No people suppressed."
          busy={busy}
          onRemove={(id) => {
            void api
              .deleteSuppression(id)
              .then(() => load())
              .catch((err: Error) => setError(err.message));
          }}
        />
      </section>

      <section className="card stack">
        <h2 style={{ margin: 0 }}>Companies</h2>
        <p className="muted">Whole companies to skip — add their domains and we won't email anyone there.</p>
        <form
          className="row-actions"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            void addDomains(domains);
          }}
        >
          <input
            value={domains}
            onChange={(e) => setDomains(e.target.value)}
            placeholder="acme.com or paste a whole list"
            style={{ minWidth: 260, flex: 1 }}
          />
          <button type="submit" disabled={busy || !domains.trim()}>
            + Add
          </button>
        </form>
        <FileDrop
          label="Or upload a CSV of domains"
          hint="One domain per line, or a domain column."
          onText={(text) => void addDomains(text)}
        />
        <SuppressionTable
          rows={companies}
          empty="No company domains suppressed."
          busy={busy}
          onRemove={(id) => {
            void api
              .deleteSuppression(id)
              .then(() => load())
              .catch((err: Error) => setError(err.message));
          }}
        />
      </section>
    </div>
  );
}

function SuppressionTable({
  rows,
  empty,
  busy,
  onRemove,
}: {
  rows: Suppression[];
  empty: string;
  busy: boolean;
  onRemove: (id: number) => void;
}) {
  return (
    <table>
      <thead>
        <tr>
          <th>Value</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {rows.length === 0 ? (
          <tr>
            <td colSpan={2} className="muted">
              {empty}
            </td>
          </tr>
        ) : (
          rows.map((s) => (
            <tr key={s.id}>
              <td>{s.value}</td>
              <td>
                <button type="button" className="secondary" disabled={busy} onClick={() => onRemove(s.id)}>
                  Remove
                </button>
              </td>
            </tr>
          ))
        )}
      </tbody>
    </table>
  );
}
