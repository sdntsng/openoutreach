import { useMemo } from "react";
import { renderFields, SAMPLE_FIELDS, type RenderedEmail, type SequenceDoc } from "./sequence";

export function SequenceEditor({
  doc,
  onChange,
  preview,
  recipientLabel,
  readOnly,
  fields,
}: {
  doc: SequenceDoc;
  onChange: (next: SequenceDoc) => void;
  preview?: RenderedEmail[] | null;
  recipientLabel?: string;
  readOnly?: boolean;
  fields?: Record<string, string>;
}) {
  const steps = doc.steps;
  const merge = fields && Object.keys(fields).length ? { ...SAMPLE_FIELDS, ...fields } : SAMPLE_FIELDS;
  const shown = useMemo(() => {
    if (preview && preview.length) return preview;
    return steps.map((st, i) => ({
      step: st.step || i + 1,
      delay: st.delay,
      subject: renderFields(st.subject, merge),
      body: renderFields(st.body, merge),
      send_note:
        i === 0
          ? "During the campaign send window after you activate."
          : `About ${st.delay} day(s) after the previous email, in the same send window.`,
    }));
  }, [preview, steps, merge]);

  function updateStep(i: number, patch: Partial<(typeof steps)[0]>) {
    const next = steps.map((s, j) => (j === i ? { ...s, ...patch } : s));
    onChange({ ...doc, steps: next.map((s, j) => ({ ...s, step: j + 1 })) });
  }

  return (
    <div className="review-grid">
      <div>
        <label>
          From name
          <input
            value={doc.from_name}
            disabled={readOnly}
            onChange={(e) => onChange({ ...doc, from_name: e.target.value })}
          />
        </label>
        {steps.map((st, i) => (
          <div key={i} className="card" style={{ marginTop: 12 }}>
            <div className="row-actions" style={{ justifyContent: "space-between" }}>
              <strong>Step {i + 1}</strong>
              {readOnly ? null : (
                <button
                  type="button"
                  className="secondary"
                  disabled={steps.length < 2}
                  onClick={() =>
                    onChange({
                      ...doc,
                      steps: steps.filter((_, j) => j !== i).map((s, j) => ({ ...s, step: j + 1 })),
                    })
                  }
                >
                  Remove
                </button>
              )}
            </div>
            <label>
              Delay (days after previous)
              <input
                type="number"
                min={0}
                disabled={readOnly}
                value={st.delay}
                onChange={(e) => updateStep(i, { delay: Math.max(0, Number(e.target.value) || 0) })}
              />
            </label>
            <label>
              Subject
              <input
                value={st.subject}
                disabled={readOnly}
                onChange={(e) => updateStep(i, { subject: e.target.value })}
              />
            </label>
            <label>
              Body
              <textarea
                rows={8}
                value={st.body}
                disabled={readOnly}
                onChange={(e) => updateStep(i, { body: e.target.value })}
              />
            </label>
            <p className="muted" style={{ fontSize: 12 }}>
              Placeholders: {"{{first_name}}"} {"{{last_name}}"} {"{{company}}"} {"{{email}}"}
            </p>
          </div>
        ))}
        {readOnly ? null : (
          <button
            type="button"
            className="secondary"
            style={{ marginTop: 8 }}
            onClick={() =>
              onChange({
                ...doc,
                steps: [
                  ...steps,
                  {
                    step: steps.length + 1,
                    delay: 3,
                    subject: "Following up",
                    body: "Hi {{first_name}},\n\nJust bumping this in case it got buried.\n\nBest",
                  },
                ],
              })
            }
          >
            Add step
          </button>
        )}
      </div>
      <div>
        <h3 style={{ marginTop: 0 }}>What they will receive</h3>
        <p className="muted" style={{ marginTop: 0 }}>
          {recipientLabel || "Preview uses Ada at Acme until you pick a recipient."}
        </p>
        {shown.length === 0 ? (
          <p className="muted">Add an email step to see the message.</p>
        ) : (
          shown.map((em) => (
            <div key={em.step} className="email-preview">
              <div className="meta">
                Step {em.step}
                {em.delay ? ` · ${em.delay}d delay` : " · first email"}
                {em.send_note ? ` · ${em.send_note}` : ""}
              </div>
              <div className="subject">{em.subject || "(no subject)"}</div>
              <pre>{em.body}</pre>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
