import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, asArray, type Account, type WorkspacePlaybook } from "../api";
import { HOURS, TIMEZONES } from "../connectors";
import { SequenceEditor } from "../SequenceEditor";
import { defaultSequence, parseSequenceYAML, sequenceToYAML, type SequenceDoc } from "../sequence";
import { FileDrop, PageIntro } from "../ui";
import { useWorkspace } from "../workspace";

type Mode = "compose" | "import";

export default function CampaignCreatePage() {
  const ws = useWorkspace();
  const navigate = useNavigate();
  const [mode, setMode] = useState<Mode>("compose");
  const [name, setName] = useState("");
  const [accountEmails, setAccountEmails] = useState<string[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [playbook, setPlaybook] = useState<WorkspacePlaybook | null>(null);
  const [csv, setCsv] = useState("email,first_name,company,title,fit_reason\n");
  const [doc, setDoc] = useState<SequenceDoc>(defaultSequence());
  const [yamlMode, setYamlMode] = useState(false);
  const [yaml, setYaml] = useState("");
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
        if (pb.default_sequence_yaml) {
          const parsed = parseSequenceYAML(pb.default_sequence_yaml);
          if (parsed && parsed.steps.length) setDoc(parsed);
          else {
            setYamlMode(true);
            setYaml(pb.default_sequence_yaml);
          }
        }
      })
      .catch(() => undefined);
  }, []);

  function toggleAccount(email: string) {
    setAccountEmails((prev) => (prev.includes(email) ? prev.filter((e) => e !== email) : [...prev, email]));
  }

  const sequenceYAML = useMemo(() => (yamlMode ? yaml : sequenceToYAML(doc)), [doc, yaml, yamlMode]);

  async function createDraft(withLeads: boolean) {
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      const created = await api.createCampaign({
        name,
        sequence_yaml: sequenceYAML,
        leads_csv: withLeads ? csv : undefined,
        accounts: accountEmails,
        draft_only: true,
        send_window_start: windowStart,
        send_window_end: windowEnd,
        timezone,
        open_tracking: openTracking,
      });
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

  async function fillFromPlaybook() {
    setBusy(true);
    setError(null);
    try {
      const d = await api.draftSequence({
        campaign_id: undefined,
        from_name: doc.from_name,
      });
      if (d.sequence_yaml) {
        const parsed = parseSequenceYAML(d.sequence_yaml);
        if (parsed) {
          setDoc(parsed);
          setYamlMode(false);
        } else {
          setYaml(d.sequence_yaml);
          setYamlMode(true);
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageIntro title="New campaign">
        Prepare your sequence. Review before sending. Connecting a mailbox is required to activate — not to draft.
      </PageIntro>
      <div className="tabs">
        <button type="button" className={mode === "compose" ? "active" : undefined} onClick={() => setMode("compose")}>
          Compose
        </button>
        <button type="button" className={mode === "import" ? "active" : undefined} onClick={() => setMode("import")}>
          Import people
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
          Write the emails first. People imported here wait on the shortlist until you approve them. Nothing
          sends until you activate.
        </p>
        <div className="row-actions">
          <button type="button" className="secondary" disabled={busy} onClick={() => void fillFromPlaybook()}>
            Draft from project facts
          </button>
          <button
            type="button"
            className="secondary"
            onClick={() => {
              if (!yamlMode) setYaml(sequenceToYAML(doc));
              else {
                const parsed = parseSequenceYAML(yaml);
                if (parsed) setDoc(parsed);
              }
              setYamlMode(!yamlMode);
            }}
          >
            {yamlMode ? "Visual editor" : "Advanced YAML"}
          </button>
        </div>
        {playbook?.audience ? <p className="muted">Project audience: {playbook.audience}</p> : null}

        {yamlMode ? (
          <label>
            Sequence YAML
            <textarea rows={14} value={yaml} onChange={(e) => setYaml(e.target.value)} />
          </label>
        ) : (
          <SequenceEditor doc={doc} onChange={setDoc} />
        )}

        {mode === "import" ? (
          <>
            <FileDrop label="Upload CSV" onText={(text) => setCsv(text)} />
            <label>
              Or paste CSV
              <textarea rows={8} value={csv} onChange={(e) => setCsv(e.target.value)} />
            </label>
            <p className="muted">Imported rows go to the shortlist. Approve them before enrollment.</p>
          </>
        ) : null}

        <AccountPicker accounts={accounts} selected={accountEmails} onToggle={toggleAccount} />
        {!ws.canSend ? (
          <p className="muted">
            You can write the emails now. <Link to="/integrations?kind=send">Connect a mailbox</Link> before
            activation.
          </p>
        ) : null}
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
          <div className="row-actions">
            <button type="button" disabled={busy || !name} onClick={(e) => void onImport(e)}>
              Create draft with people
            </button>
            <Link to="/campaigns" className="muted">
              Cancel
            </Link>
          </div>
        ) : (
          <div className="row-actions">
            <button type="submit" disabled={busy || !name}>
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
      <legend>Sending accounts (optional until activate)</legend>
      {accounts.length === 0 ? (
        <p className="muted">
          No mailbox yet. You can still prepare the sequence. Connect one on{" "}
          <Link to="/integrations?kind=send">Integrations</Link> when you are ready to send.
        </p>
      ) : (
        accounts.map((a) => (
          <label key={a.id} className="row">
            <input type="checkbox" checked={selected.includes(a.email)} onChange={() => onToggle(a.email)} />
            {a.email}
            <span className="muted" style={{ marginLeft: 8 }}>
              {connectionLabel(a.oauth_health, a.reply_mode)}
            </span>
          </label>
        ))
      )}
    </fieldset>
  );
}

function connectionLabel(health?: string, replyMode?: string): string {
  const h =
    health === "reconnect_required"
      ? "Reconnect required"
      : health === "verified"
        ? "Verified"
        : health === "saved"
          ? "Saved; connection not verified"
          : "";
  const r = replyMode === "send_only" ? "send only — no reply inbox" : "";
  return [h, r].filter(Boolean).join(" · ");
}
