import { useEffect, useState } from "react";
import { api, type WorkspacePlaybook } from "../api";
import { DEFAULT_SEQUENCE } from "../defaults";
import { PageIntro } from "../ui";

const CHIPS = [
  "Ask for a reply, not a call",
  "Keep it short",
  "Mention our offer",
  "No discounts",
  "Professional tone",
];

export default function TemplatesPage() {
  const [pb, setPb] = useState<WorkspacePlaybook>({});
  const [preview, setPreview] = useState<unknown>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .getPlaybook()
      .then((p) => {
        if (!p.default_sequence_yaml) p.default_sequence_yaml = DEFAULT_SEQUENCE;
        setPb(p);
      })
      .catch((err: Error) => setError(err.message));
  }, []);

  async function save() {
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

  async function loadPreview() {
    setBusy(true);
    setError(null);
    try {
      const d = await api.draftSequence({
        icp: pb.audience || pb.company || "your ICP",
        offer: pb.offer || "our product",
        tone: pb.template_instructions || "direct",
      });
      setPreview(d.preview || d);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageIntro title="Email templates">
        Default sequence for new campaigns. Templates are ReplaceAll YAML — review, then activate the campaign
        separately.
      </PageIntro>
      {error && <div className="error">{error}</div>}
      <div className="split-wide">
        <div className="card stack">
          <label>
            Instructions (optional)
            <textarea
              rows={5}
              value={pb.template_instructions || ""}
              onChange={(e) => setPb({ ...pb, template_instructions: e.target.value })}
            />
          </label>
          <div className="pill-row">
            {CHIPS.map((c) => (
              <button
                key={c}
                type="button"
                className="chip"
                onClick={() =>
                  setPb({
                    ...pb,
                    template_instructions: [pb.template_instructions, c].filter(Boolean).join(". "),
                  })
                }
              >
                + {c}
              </button>
            ))}
          </div>
          <label>
            Default sequence YAML
            <textarea
              rows={16}
              value={pb.default_sequence_yaml || ""}
              onChange={(e) => setPb({ ...pb, default_sequence_yaml: e.target.value })}
            />
          </label>
          <div className="row-actions">
            <button type="button" disabled={busy} onClick={() => void save()}>
              Save templates
            </button>
            <button type="button" className="secondary" disabled={busy} onClick={() => void loadPreview()}>
              Preview sample
            </button>
          </div>
        </div>
        <div className="card stack">
          <h2 style={{ margin: 0 }}>What leads get</h2>
          <p className="muted">Sample render with Ada @ Acme. Placeholders: first_name, company, email.</p>
          {preview ? (
            <pre className="code">{JSON.stringify(preview, null, 2)}</pre>
          ) : (
            <p className="muted">Save a sequence, then preview.</p>
          )}
        </div>
      </div>
    </div>
  );
}
