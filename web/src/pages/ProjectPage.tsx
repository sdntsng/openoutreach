import { useEffect, useState, type FormEvent } from "react";
import { api, type WorkspacePlaybook } from "../api";
import { PageIntro, PillList } from "../ui";

export default function ProjectPage() {
  const [pb, setPb] = useState<WorkspacePlaybook>({});
  const [competitor, setCompetitor] = useState("");
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
      <PageIntro title="Project">
        Everything the agent and templates can use: company profile, offer, and competitors. Saved to this
        workspace — never sends mail.
      </PageIntro>
      {error && <div className="error">{error}</div>}
      <form className="card stack" onSubmit={(e) => void save(e)}>
        <div className="field-row">
          <label>
            Company
            <input value={pb.company || ""} onChange={(e) => setPb({ ...pb, company: e.target.value })} />
          </label>
          <label>
            Website
            <input value={pb.website || ""} onChange={(e) => setPb({ ...pb, website: e.target.value })} />
          </label>
          <label>
            Location
            <input value={pb.location || ""} onChange={(e) => setPb({ ...pb, location: e.target.value })} />
          </label>
        </div>
        <label>
          Description
          <textarea rows={4} value={pb.description || ""} onChange={(e) => setPb({ ...pb, description: e.target.value })} />
        </label>
        <label>
          What you offer
          <textarea rows={3} value={pb.offer || ""} onChange={(e) => setPb({ ...pb, offer: e.target.value })} />
        </label>
        <label>
          Customer problem
          <textarea rows={3} value={pb.problem || ""} onChange={(e) => setPb({ ...pb, problem: e.target.value })} />
        </label>
        <label>
          Audience
          <textarea rows={3} value={pb.audience || ""} onChange={(e) => setPb({ ...pb, audience: e.target.value })} />
        </label>
        <div>
          <div className="muted" style={{ marginBottom: "0.35rem" }}>
            Competitors
          </div>
          <div className="row-actions">
            <input
              value={competitor}
              onChange={(e) => setCompetitor(e.target.value)}
              placeholder="competitor.com"
            />
            <button
              type="button"
              className="secondary"
              onClick={() => {
                const v = competitor.trim();
                if (!v) return;
                setPb({ ...pb, competitors: [...(pb.competitors || []), v] });
                setCompetitor("");
              }}
            >
              + Add
            </button>
          </div>
          <PillList
            items={pb.competitors || []}
            onRemove={(v) => setPb({ ...pb, competitors: (pb.competitors || []).filter((c) => c !== v) })}
          />
        </div>
        <button type="submit" disabled={busy}>
          Save project
        </button>
      </form>
    </div>
  );
}
