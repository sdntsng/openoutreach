import { useEffect, useMemo, useState } from "react";
import { api, type WorkspacePlaybook } from "../api";
import { DEFAULT_SEQUENCE } from "../defaults";
import { SequenceEditor } from "../SequenceEditor";
import { parseSequenceYAML, sequenceToYAML, type RenderedEmail, type SequenceDoc } from "../sequence";
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
  const [doc, setDoc] = useState<SequenceDoc | null>(null);
  const [yamlMode, setYamlMode] = useState(false);
  const [preview, setPreview] = useState<RenderedEmail[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .getPlaybook()
      .then((p) => {
        if (!p.default_sequence_yaml) p.default_sequence_yaml = DEFAULT_SEQUENCE;
        setPb(p);
        const parsed = parseSequenceYAML(p.default_sequence_yaml);
        if (parsed) setDoc(parsed);
        else setYamlMode(true);
      })
      .catch((err: Error) => setError(err.message));
  }, []);

  const yaml = useMemo(() => {
    if (yamlMode) return pb.default_sequence_yaml || "";
    return doc ? sequenceToYAML(doc) : pb.default_sequence_yaml || "";
  }, [doc, yamlMode, pb.default_sequence_yaml]);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      const next = { ...pb, default_sequence_yaml: yamlMode ? pb.default_sequence_yaml : sequenceToYAML(doc || { name: "outreach", from_name: "You", steps: [] }) };
      setPb(await api.putPlaybook(next));
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
      const d = await api.previewSequence({ sequence_yaml: yaml });
      setPreview(d.rendered || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageIntro title="Email templates">
        Default sequence for new campaigns. Edit the emails you will send. YAML is available as an advanced view over
        the same format. Activate still happens on the campaign, with confirm.
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
          <div className="row-actions">
            <button
              type="button"
              className="secondary"
              onClick={() => {
                if (!yamlMode && doc) setPb({ ...pb, default_sequence_yaml: sequenceToYAML(doc) });
                else {
                  const parsed = parseSequenceYAML(pb.default_sequence_yaml || "");
                  if (parsed) setDoc(parsed);
                }
                setYamlMode(!yamlMode);
              }}
            >
              {yamlMode ? "Visual editor" : "Advanced YAML"}
            </button>
          </div>
          {yamlMode || !doc ? (
            <label>
              Default sequence YAML
              <textarea
                rows={16}
                value={pb.default_sequence_yaml || ""}
                onChange={(e) => setPb({ ...pb, default_sequence_yaml: e.target.value })}
              />
            </label>
          ) : (
            <SequenceEditor doc={doc} onChange={setDoc} />
          )}
          <div className="row-actions">
            <button type="button" disabled={busy} onClick={() => void save()}>
              Save templates
            </button>
            <button type="button" className="secondary" disabled={busy} onClick={() => void loadPreview()}>
              Preview this sequence
            </button>
          </div>
        </div>
        {yamlMode ? (
          <div className="card stack">
            <h2 style={{ margin: 0 }}>What leads get</h2>
            <p className="muted">Rendered from the YAML you are editing, with Ada @ Acme.</p>
            {preview?.length ? (
              preview.map((em) => (
                <div key={em.step} className="email-preview">
                  <div className="meta">Step {em.step}</div>
                  <div className="subject">{em.subject}</div>
                  <pre>{em.body}</pre>
                </div>
              ))
            ) : (
              <p className="muted">Save a sequence, then preview.</p>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}
