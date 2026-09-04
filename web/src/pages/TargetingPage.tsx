import { useEffect, useState, type FormEvent } from "react";
import { api, type WorkspacePlaybook } from "../api";
import { PageIntro } from "../ui";

const GEO = ["United States", "English-speaking countries", "DACH", "EU + UK", "high-income markets"];
const SIZE = ["Startups (1–50)", "Mid-market (51–500)", "Enterprise (500+)", "SMBs under 200", "exclude solopreneurs"];

function addPhrase(cur: string | undefined, phrase: string): string {
  const c = (cur || "").trim();
  if (c.includes(phrase)) return c;
  return c ? `${c}, ${phrase}` : phrase;
}

export default function TargetingPage() {
  const [pb, setPb] = useState<WorkspacePlaybook>({});
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .getPlaybook()
      .then(setPb)
      .catch((err: Error) => setError(err.message));
  }, []);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      setPb(await api.putPlaybook(pb));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageIntro title="Targeting">
        ICP notes for this workspace. We do not run a paid lead graph — import CSV, Apollo, Sheets, or Clay.
      </PageIntro>
      {error && <div className="error">{error}</div>}
      <form className="card stack" onSubmit={(e) => void save(e)}>
        <label>
          Geography
          <textarea rows={4} value={pb.geography || ""} onChange={(e) => setPb({ ...pb, geography: e.target.value })} />
        </label>
        <div className="pill-row">
          {GEO.map((g) => (
            <button key={g} type="button" className="chip" onClick={() => setPb({ ...pb, geography: addPhrase(pb.geography, g) })}>
              + {g}
            </button>
          ))}
        </div>
        <label>
          Company size
          <textarea
            rows={4}
            value={pb.company_size || ""}
            onChange={(e) => setPb({ ...pb, company_size: e.target.value })}
          />
        </label>
        <div className="pill-row">
          {SIZE.map((g) => (
            <button
              key={g}
              type="button"
              className="chip"
              onClick={() => setPb({ ...pb, company_size: addPhrase(pb.company_size, g) })}
            >
              + {g}
            </button>
          ))}
        </div>
        <button type="submit" disabled={busy}>
          Save targeting
        </button>
      </form>
    </div>
  );
}
