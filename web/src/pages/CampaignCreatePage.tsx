import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, asArray, type Account, type WorkspacePlaybook } from "../api";
import { HOURS, TIMEZONES } from "../connectors";
import { DEFAULT_SEQUENCE } from "../defaults";
import { FileDrop, PageIntro } from "../ui";

type Mode = "compose" | "import";

export default function CampaignCreatePage() {
  const navigate = useNavigate();
  const [mode, setMode] = useState<Mode>("compose");
  const [name, setName] = useState("");
  const [accountEmails, setAccountEmails] = useState<string[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [playbook, setPlaybook] = useState<WorkspacePlaybook | null>(null);
  const [csv, setCsv] = useState("email,first_name,company\n");
  const [sequence, setSequence] = useState(DEFAULT_SEQUENCE);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
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
    api
      .getPlaybook()
      .then((pb) => {
        setPlaybook(pb);
        if (pb.send_window_start) setWindowStart(pb.send_window_start);
        if (pb.send_window_end) setWindowEnd(pb.send_window_end);
        if (pb.timezone) setTimezone(pb.timezone);
        if (pb.default_sequence_yaml) setSequence(pb.default_sequence_yaml);
      })
      .catch(() => undefined);
  }, []);

  function toggleAccount(email: string) {
    setAccountEmails((prev) => (prev.includes(email) ? prev.filter((e) => e !== email) : [...prev, email]));
  }

  async function createDraft(withLeads: boolean) {
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      const created = await api.createCampaign({
        name,
        sequence_yaml: withLeads ? sequence : sequence || playbook?.default_sequence_yaml,
        leads_csv: withLeads ? csv : undefined,
        accounts: accountEmails,
        draft_only: !withLeads,
        send_window_start: windowStart,
        send_window_end: windowEnd,
        timezone,
        open_tracking: openTracking,
      });
      if (!withLeads && (playbook?.offer || name)) {
        await api
          .draftSequence({
            icp: name,
            offer: playbook?.offer || name,
            campaign_id: created.campaign_id,
          })
          .catch(() => undefined);
      }
      navigate(`/campaigns/${created.campaign_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function onImport(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const v = await api.validateLeads({ csv });
      setNote(`total ${v.total} · valid ${v.valid} · invalid ${v.invalid} · duplicate ${v.duplicate}`);
      if (v.invalid > 0) throw new Error("Fix invalid leads before creating");
      await createDraft(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  return (
    <div>
      <PageIntro title="New campaign">
        Create stays draft. Activate on the campaign page is a separate, explicit confirm.
      </PageIntro>
      <div className="tabs">
        <button type="button" className={mode === "compose" ? "active" : undefined} onClick={() => setMode("compose")}>
          Compose
        </button>
        <button type="button" className={mode === "import" ? "active" : undefined} onClick={() => setMode("import")}>
          Import leads
        </button>
      </div>
      {error && <p className="error">{error}</p>}
      {note && <p className="muted">{note}</p>}

      <form
        className="card stack"
        onSubmit={(e) => {
          e.preventDefault();
          if (mode === "compose") void createDraft(false);
        }}
      >
        <label>
          Campaign name
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Mid-market SaaS founders"
            required
          />
        </label>
        <p className="muted">
          Name the audience. Sequence defaults come from Email templates / Project — you review YAML before activate.
        </p>
        <AccountPicker accounts={accounts} selected={accountEmails} onToggle={toggleAccount} />
        <div className="field-row">
          <label>
            Window start
            <select value={windowStart} onChange={(e) => setWindowStart(e.target.value)}>
              {HOURS.map((h) => (
                <option key={`s-${h}`} value={h}>
                  {h}
                </option>
              ))}
            </select>
          </label>
          <label>
            Window end
            <select value={windowEnd} onChange={(e) => setWindowEnd(e.target.value)}>
              {HOURS.map((h) => (
                <option key={`e-${h}`} value={h}>
                  {h}
                </option>
              ))}
            </select>
          </label>
          <label>
            Timezone
            <select value={timezone} onChange={(e) => setTimezone(e.target.value)}>
              {TIMEZONES.map((tz) => (
                <option key={tz} value={tz}>
                  {tz}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label>
          Approx. open tracking
          <select value={openTracking ? "on" : "off"} onChange={(e) => setOpenTracking(e.target.value === "on")}>
            <option value="off">Off</option>
            <option value="on">On (pixel; never blocks send)</option>
          </select>
        </label>

        {mode === "import" ? (
          <>
            <FileDrop label="Upload CSV" onText={(text) => setCsv(text)} />
            <label>
              Or paste CSV
              <textarea rows={8} value={csv} onChange={(e) => setCsv(e.target.value)} />
            </label>
            <label>
              Sequence YAML
              <textarea rows={10} value={sequence} onChange={(e) => setSequence(e.target.value)} />
            </label>
            <div className="row-actions">
              <button type="button" disabled={busy || !name || accountEmails.length === 0} onClick={(e) => void onImport(e)}>
                Create draft with leads
              </button>
              <Link to="/campaigns" className="muted">
                Cancel
              </Link>
            </div>
          </>
        ) : (
          <div className="row-actions">
            <button type="submit" disabled={busy || !name || accountEmails.length === 0}>
              Create draft campaign
            </button>
            <Link to="/campaigns" className="muted">
              Cancel
            </Link>
          </div>
        )}
      </form>
    </div>
  );
}

function AccountPicker({
  accounts,
  selected,
  onToggle,
}: {
  accounts: Account[];
  selected: string[];
  onToggle: (email: string) => void;
}) {
  return (
    <fieldset>
      <legend>Sending accounts</legend>
      {accounts.length === 0 ? (
        <p className="muted">
          Connect a mailbox on <Link to="/integrations?kind=send">Integrations</Link> first.
        </p>
      ) : (
        accounts.map((a) => (
          <label key={a.id} className="row">
            <input type="checkbox" checked={selected.includes(a.email)} onChange={() => onToggle(a.email)} />
            {a.email}
          </label>
        ))
      )}
    </fieldset>
  );
}
