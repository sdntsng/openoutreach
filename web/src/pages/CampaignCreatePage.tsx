import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, asArray, type Account } from "../api";

type Step = "details" | "leads" | "sequence" | "preview" | "activate";
const STEPS: Step[] = ["details", "leads", "sequence", "preview", "activate"];

const DEFAULT_SEQUENCE = `name: outreach
defaults:
  from_name: "You"
steps:
  - step: 1
    delay: 0
    subject: "Quick question for {{company}}"
    body: |
      Hi {{first_name}},

      Wanted to reach out about {{company}}.
  - step: 2
    delay: 3
    body: |
      Hi {{first_name}},

      Following up once in case this got buried.
`;

export default function CampaignCreatePage() {
  const navigate = useNavigate();
  const [step, setStep] = useState<Step>("details");
  const [name, setName] = useState("");
  const [accountEmails, setAccountEmails] = useState<string[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [csv, setCsv] = useState("email,first_name,company\n");
  const [sequence, setSequence] = useState(DEFAULT_SEQUENCE);
  const [campaignId, setCampaignId] = useState<string | number | null>(null);
  const [preview, setPreview] = useState<unknown>(null);
  const [validation, setValidation] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [windowStart, setWindowStart] = useState("09:00");
  const [windowEnd, setWindowEnd] = useState("17:00");
  const [timezone, setTimezone] = useState("UTC");
  const [openTracking, setOpenTracking] = useState(false);

  useEffect(() => {
    api
      .listAccounts()
      .then((data) => setAccounts(asArray(data, "accounts")))
      .catch(() => setAccounts([]));
  }, []);

  const stepIndex = STEPS.indexOf(step);
  const stepLabels = useMemo(
    () =>
      STEPS.map((s, i) => (
        <span key={s} className={i < stepIndex ? "done" : i === stepIndex ? "current" : ""}>
          {i + 1}. {s}
          {i < STEPS.length - 1 ? " → " : ""}
        </span>
      )),
    [stepIndex],
  );

  function toggleAccount(email: string) {
    setAccountEmails((prev) =>
      prev.includes(email) ? prev.filter((e) => e !== email) : [...prev, email],
    );
  }

  async function onLeads(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const v = await api.validateLeads({ csv });
      setValidation(
        `total ${v.total} · valid ${v.valid} · invalid ${v.invalid} · duplicate ${v.duplicate}`,
      );
      if (v.invalid > 0) throw new Error("Fix invalid leads before continuing");
      setStep("sequence");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onSequence(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const created = await api.createCampaign({
        name,
        sequence_yaml: sequence,
        leads_csv: csv,
        accounts: accountEmails,
        draft_only: false,
        send_window_start: windowStart,
        send_window_end: windowEnd,
        timezone,
        open_tracking: openTracking,
      });
      setCampaignId(created.campaign_id);
      setStep("preview");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function loadPreview() {
    if (campaignId == null) return;
    setBusy(true);
    try {
      setPreview(await api.getCampaignPreview(campaignId));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onActivate() {
    if (campaignId == null) return;
    setBusy(true);
    try {
      await api.activateCampaign(campaignId);
      navigate(`/campaigns/${campaignId}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>New campaign</h1>
        <Link to="/campaigns">Back</Link>
      </div>
      <p className="muted">{stepLabels}</p>
      {error && <p className="error">{error}</p>}
      {validation && <p className="muted">{validation}</p>}

      {step === "details" && (
        <form
          className="card stack"
          onSubmit={(e) => {
            e.preventDefault();
            setStep("leads");
          }}
        >
          <label>
            Campaign name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Send window start
            <input value={windowStart} onChange={(e) => setWindowStart(e.target.value)} placeholder="09:00" />
          </label>
          <label>
            Send window end
            <input value={windowEnd} onChange={(e) => setWindowEnd(e.target.value)} placeholder="17:00" />
          </label>
          <label>
            Timezone
            <input value={timezone} onChange={(e) => setTimezone(e.target.value)} placeholder="UTC" />
          </label>
          <label className="row">
            <input
              type="checkbox"
              checked={openTracking}
              onChange={(e) => setOpenTracking(e.target.checked)}
            />
            Approx. open tracking (pixel; never blocks send)
          </label>
          <fieldset>
            <legend>Sending accounts</legend>
            {accounts.map((a) => (
              <label key={a.id} className="row">
                <input
                  type="checkbox"
                  checked={accountEmails.includes(a.email)}
                  onChange={() => toggleAccount(a.email)}
                />
                {a.email}
              </label>
            ))}
          </fieldset>
          <button type="submit" disabled={!name || accountEmails.length === 0}>
            Continue to leads
          </button>
        </form>
      )}

      {step === "leads" && (
        <form className="card stack" onSubmit={onLeads}>
          <label>
            Leads CSV
            <textarea rows={10} value={csv} onChange={(e) => setCsv(e.target.value)} />
          </label>
          <button type="submit" disabled={busy}>
            Validate & continue
          </button>
        </form>
      )}

      {step === "sequence" && (
        <form className="card stack" onSubmit={onSequence}>
          <label>
            Sequence YAML
            <textarea rows={16} value={sequence} onChange={(e) => setSequence(e.target.value)} />
          </label>
          <button type="submit" disabled={busy}>
            Create draft campaign
          </button>
        </form>
      )}

      {step === "preview" && (
        <div className="card stack">
          <p>Campaign remains <strong>draft</strong> until you activate.</p>
          <button type="button" onClick={loadPreview} disabled={busy}>
            Load preview
          </button>
          <pre className="code">{preview ? JSON.stringify(preview, null, 2) : "—"}</pre>
          <button type="button" onClick={() => setStep("activate")}>
            Continue to activate
          </button>
        </div>
      )}

      {step === "activate" && (
        <div className="card stack">
          <p>
            <strong>Consequential:</strong> Activate starts sending due emails via the tick engine.
          </p>
          <button type="button" className="danger" onClick={onActivate} disabled={busy}>
            Activate campaign
          </button>
        </div>
      )}
    </div>
  );
}
