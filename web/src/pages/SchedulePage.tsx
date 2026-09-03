import { useEffect, useState, type FormEvent } from "react";
import { api, type WorkspacePlaybook } from "../api";
import { HOURS, TIMEZONES } from "../connectors";
import { PageIntro } from "../ui";

const DAYS = [
  { id: "1", label: "Mon" },
  { id: "2", label: "Tue" },
  { id: "3", label: "Wed" },
  { id: "4", label: "Thu" },
  { id: "5", label: "Fri" },
  { id: "6", label: "Sat" },
  { id: "0", label: "Sun" },
];

export default function SchedulePage() {
  const [pb, setPb] = useState<WorkspacePlaybook>({});
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .getPlaybook()
      .then(setPb)
      .catch((err: Error) => setError(err.message));
  }, []);

  const selected = new Set((pb.send_days || "1,2,3,4,5").split(",").map((s) => s.trim()).filter(Boolean));

  function toggleDay(id: string) {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setPb({ ...pb, send_days: DAYS.map((d) => d.id).filter((d) => next.has(d)).join(",") });
  }

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
      <PageIntro title="Sending schedule">
        Default windows for new campaigns. Existing campaigns keep their own windows until you edit them.
      </PageIntro>
      {error && <div className="error">{error}</div>}
      <form className="card stack" onSubmit={(e) => void save(e)} style={{ maxWidth: 560 }}>
        <div className="field-row">
          <label>
            Start
            <select
              value={pb.send_window_start || "09:00"}
              onChange={(e) => setPb({ ...pb, send_window_start: e.target.value })}
            >
              {HOURS.map((h) => (
                <option key={h} value={h}>
                  {h}
                </option>
              ))}
            </select>
          </label>
          <label>
            End
            <select
              value={pb.send_window_end || "17:00"}
              onChange={(e) => setPb({ ...pb, send_window_end: e.target.value })}
            >
              {HOURS.map((h) => (
                <option key={h} value={h}>
                  {h}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label>
          Timezone
          <select value={pb.timezone || "UTC"} onChange={(e) => setPb({ ...pb, timezone: e.target.value })}>
            {TIMEZONES.map((tz) => (
              <option key={tz} value={tz}>
                {tz}
              </option>
            ))}
          </select>
        </label>
        <div>
          <div className="muted" style={{ marginBottom: "0.4rem" }}>
            Send days
          </div>
          <div className="pill-row">
            {DAYS.map((d) => (
              <button
                key={d.id}
                type="button"
                className={selected.has(d.id) ? "chip is-on" : "chip"}
                onClick={() => toggleDay(d.id)}
              >
                {d.label}
              </button>
            ))}
          </div>
        </div>
        <button type="submit" disabled={busy}>
          Save schedule
        </button>
      </form>
    </div>
  );
}
