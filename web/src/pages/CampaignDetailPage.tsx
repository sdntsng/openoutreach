import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ApiError, api, asArray, type Account, type CampaignReview, type CampaignStats } from "../api";
import { LeadImport } from "../LeadImport";
import { SequenceEditor } from "../SequenceEditor";
import { defaultSequence, parseSequenceYAML, sequenceToYAML, type SequenceDoc } from "../sequence";
import { StatusChip } from "../ui";
import { GATES, useWorkspace } from "../workspace";

type Tab = "review" | "recipients" | "yaml";

const OPEN_TOOLTIP =
  "Approx. opens are inferred from tracking pixel loads. Image proxies and privacy features can skew this metric.";

function str(v: unknown, fallback = "—"): string {
  if (v == null) return fallback;
  return String(v);
}

function downloadCSV(filename: string, csv: string) {
  const blob = new Blob([csv], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export default function CampaignDetailPage() {
  const ws = useWorkspace();
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const tab = (params.get("tab") as Tab) || "review";
  const [review, setReview] = useState<CampaignReview | null>(null);
  const [stats, setStats] = useState<CampaignStats | null>(null);
  const [doc, setDoc] = useState<SequenceDoc>(defaultSequence());
  const [yaml, setYaml] = useState("");
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [selectedAccounts, setSelectedAccounts] = useState<string[]>([]);
  const [previewLead, setPreviewLead] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function setTab(next: Tab) {
    const q = new URLSearchParams(params);
    if (next === "review") q.delete("tab");
    else q.set("tab", next);
    setParams(q, { replace: true });
  }

  async function reload() {
    const rev = await api.campaignReview(id, previewLead || undefined);
    setReview(rev);
    setYaml(rev.sequence?.yaml || "");
    if (rev.sequence?.steps?.length) {
      setDoc({
        name: rev.sequence.name || "outreach",
        from_name: rev.sequence.from_name || "You",
        steps: rev.sequence.steps.map((s) => ({
          step: s.step,
          delay: s.delay,
          subject: s.subject,
          body: s.body,
        })),
      });
    } else {
      const parsed = parseSequenceYAML(rev.sequence?.yaml || "");
      setDoc(parsed || defaultSequence());
    }
    setSelectedAccounts(rev.accounts || []);
    try {
      setStats(await api.getCampaignStats(id));
    } catch {
      setStats(null);
    }
  }

  useEffect(() => {
    setError(null);
    reload().catch((err: Error) => setError(err.message));
    api
      .listAccounts()
      .then((data) => setAccounts(asArray(data, "accounts")))
      .catch(() => setAccounts([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, previewLead]);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await action();
      await reload();
    } catch (err) {
      if (err instanceof ApiError) {
        const data = err.body as { data?: CampaignReview; error?: { message?: string } };
        if (data?.data?.checklist) setReview(data.data);
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setBusy(false);
    }
  }

  const status = review?.status || "";
  const name = review?.name || "Campaign";
  const leadCount = review?.enrolled ?? 0;
  const editable = status === "draft" || status === "paused";
  const visualOK = useMemo(() => parseSequenceYAML(yaml) !== null, [yaml]);
  const recipientLabel = review?.preview_lead?.email
    ? `${review.preview_lead.first_name || review.preview_lead.email} at ${review.preview_lead.company || "—"} (${review.preview_lead.source || "preview"})`
    : undefined;

  if (!review && !error) return <p className="muted">Loading…</p>;

  return (
    <div>
      <div className="row-actions" style={{ marginBottom: "0.5rem" }}>
        <Link to="/campaigns">← Campaigns</Link>
        <Link to={`/shortlist?campaign=${id}`}>Shortlist</Link>
      </div>
      <div className="row-actions" style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
        <div>
          <h1 style={{ margin: 0 }}>{name}</h1>
          <p className="status-banner">
            <StatusChip status={status} />{" "}
            {status === "paused"
              ? "Sending stopped. Replies are still monitored."
              : status === "draft"
                ? "Draft — nothing sends until you review and activate."
                : status === "active"
                  ? review?.next_send_note || "Active — due mail sends in the next tick."
                  : null}
          </p>
          {review?.next_send_note && status !== "active" ? <p className="muted">{review.next_send_note}</p> : null}
        </div>
        <div className="row-actions">
          {status === "draft" && !ws.canSend && (
            <Link to={GATES.sender.to}>
              <button type="button" className="secondary">
                Connect a sending account
              </button>
            </Link>
          )}
          {status === "draft" && (
            <button
              type="button"
              disabled={busy || !review?.ready}
              onClick={() => {
                const n = review?.enrolled ?? 0;
                const acc = (review?.accounts || []).join(", ") || "none";
                if (
                  !window.confirm(
                    `Activate and start sending?\n\n${n} recipient(s)\nFrom: ${acc}\nWindow: ${review?.send_window} ${review?.timezone}`,
                  )
                ) {
                  return;
                }
                void run(() => api.activateCampaign(id));
              }}
            >
              Activate
            </button>
          )}
          {status === "active" && (
            <button type="button" className="secondary" disabled={busy} onClick={() => void run(() => api.pauseCampaign(id))}>
              Pause
            </button>
          )}
          {status === "paused" && (
            <button type="button" disabled={busy || !ws.canSend} onClick={() => void run(() => api.resumeCampaign(id))}>
              Resume
            </button>
          )}
          <button
            type="button"
            className="secondary"
            disabled={busy}
            onClick={() => {
              const next = window.prompt("Clone as draft. New name?", `${name}-copy`);
              if (!next) return;
              setBusy(true);
              setError(null);
              api
                .cloneCampaign(id, { name: next })
                .then((res) => navigate(`/campaigns/${res.campaign_id}`))
                .catch((err: Error) => setError(err.message))
                .finally(() => setBusy(false));
            }}
          >
            Clone
          </button>
        </div>
      </div>
      {error && <div className="error">{error}</div>}

      {review?.checklist?.length ? (
        <div className="card checklist">
          <h2 style={{ marginTop: 0 }}>Before you activate</h2>
          <ul className="checklist-list">
            {review.checklist.map((item) => (
              <li key={item.id} className={item.ok ? "is-ok" : "is-block"}>
                <span>{item.ok ? "Ready" : "Needs fix"}</span> {item.label}
                {!item.ok && item.fix ? (
                  <Link to={item.fix} style={{ marginLeft: 8 }}>
                    Fix
                  </Link>
                ) : null}
              </li>
            ))}
          </ul>
          {review.warnings?.length ? (
            <ul className="muted">
              {review.warnings.map((w) => (
                <li key={w}>{w}</li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}

      <div className="tabs">
        <button type="button" className={tab === "review" ? "active" : undefined} onClick={() => setTab("review")}>
          Review
        </button>
        <button type="button" className={tab === "recipients" ? "active" : undefined} onClick={() => setTab("recipients")}>
          Recipients{leadCount ? ` (${leadCount})` : ""}
          {review?.pending_review ? ` · ${review.pending_review} to review` : ""}
        </button>
        <button type="button" className={tab === "yaml" ? "active" : undefined} onClick={() => setTab("yaml")}>
          Advanced YAML
        </button>
      </div>

      {tab === "review" && review && (
        <div className="stack">
          {stats && status !== "draft" ? (
            <div className="metrics">
              <div className="metric">
                <div className="label">Sent</div>
                <div className="value">{stats.sent ?? 0}</div>
              </div>
              <div className="metric">
                <div className="label">Replies</div>
                <div className="value">{stats.replies ?? 0}</div>
              </div>
              <div className="metric prominent">
                <div className="label">Reply rate</div>
                <div className="value">
                  {stats.reply_rate != null
                    ? `${(stats.reply_rate <= 1 ? stats.reply_rate * 100 : stats.reply_rate).toFixed(1)}%`
                    : "—"}
                </div>
              </div>
              <div className="metric" title={OPEN_TOOLTIP}>
                <div className="label">Approx. opens</div>
                <div className="value">{stats.approx_opens ?? 0}</div>
              </div>
            </div>
          ) : null}
          <div className="card stack">
            <p className="muted">
              Window {review.send_window} {review.timezone} · {review.send_days} ·{" "}
              {review.accounts?.length ? review.accounts.join(", ") : "no sending account yet"}
            </p>
            <label>
              Preview as
              <select value={previewLead} onChange={(e) => setPreviewLead(e.target.value)}>
                <option value="">Sample (Ada @ Acme)</option>
                {(review.preview_options || review.recipients || []).map((r) => (
                  <option key={r.email} value={r.email}>
                    {r.first_name || r.email} · {r.email}
                    {r.source ? ` (${r.source})` : ""}
                  </option>
                ))}
              </select>
            </label>
            <fieldset>
              <legend>Sending accounts</legend>
              {accounts.length === 0 ? (
                <p className="muted">
                  <Link to="/integrations?kind=send">Connect a mailbox</Link> to assign one.
                </p>
              ) : (
                accounts.map((a) => (
                  <label key={a.id} className="row">
                    <input
                      type="checkbox"
                      disabled={!editable}
                      checked={selectedAccounts.includes(a.email)}
                      onChange={() =>
                        setSelectedAccounts((prev) =>
                          prev.includes(a.email) ? prev.filter((e) => e !== a.email) : [...prev, a.email],
                        )
                      }
                    />
                    {a.email}
                  </label>
                ))
              )}
            </fieldset>
            {visualOK ? (
              <SequenceEditor
                doc={doc}
                onChange={setDoc}
                preview={!editable ? review.rendered : undefined}
                fields={{
                  first_name: review.preview_lead?.first_name || "",
                  company: review.preview_lead?.company || "",
                  email: review.preview_lead?.email || "",
                }}
                recipientLabel={recipientLabel}
                readOnly={!editable}
              />
            ) : (
              <p className="muted">This sequence uses variants. Edit it in Advanced YAML so we do not flatten it.</p>
            )}
            {editable ? (
              <button
                type="button"
                disabled={busy}
                onClick={() =>
                  void run(() =>
                    api.patchCampaign(id, {
                      sequence_yaml: visualOK ? sequenceToYAML(doc) : yaml,
                      accounts: selectedAccounts,
                    }),
                  )
                }
              >
                Save sequence
              </button>
            ) : (
              <p className="muted">Pause the campaign to edit the sequence.</p>
            )}
          </div>
          {review.exclusions?.length ? (
            <p className="muted">Exclusions: {review.exclusions.slice(0, 8).join(" · ")}</p>
          ) : null}
        </div>
      )}

      {tab === "recipients" && (
        <div className="stack">
          <p className="muted">
            {review?.pending_review
              ? `${review.pending_review} people are waiting on the shortlist. Approve them before they can be enrolled.`
              : "Enrolled people are the ones who will receive this sequence."}{" "}
            <Link to={`/shortlist?campaign=${id}`}>Open shortlist</Link>
          </p>
          <div className="row-actions" style={{ justifyContent: "flex-end" }}>
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={() => {
                void api
                  .exportCampaignLeads(id)
                  .then((res) => downloadCSV(`${name}-leads.csv`, res.csv))
                  .catch((err: Error) => setError(err.message));
              }}
            >
              Export enrolled
            </button>
          </div>
          <LeadImport campaignId={id} onImported={() => void reload()} destination="shortlist" />
          {review?.recipients?.length ? (
            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Name</th>
                  <th>Company</th>
                  <th>Source</th>
                </tr>
              </thead>
              <tbody>
                {review.recipients.map((row) => (
                  <tr key={row.email}>
                    <td>{row.email}</td>
                    <td>{row.first_name || "—"}</td>
                    <td>{row.company || "—"}</td>
                    <td>{row.source || "enrolled"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : Array.isArray(stats?.leads) && stats && stats.leads.length > 0 ? (
            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Status</th>
                  <th>Steps sent</th>
                  <th>Reply</th>
                </tr>
              </thead>
              <tbody>
                {(stats.leads as Array<Record<string, unknown>>).map((row, i) => (
                  <tr key={String(row.email || i)}>
                    <td>{str(row.email)}</td>
                    <td>{str(row.status)}</td>
                    <td>{str(row.steps_sent ?? row.sent)}</td>
                    <td>{row.reply_at ? "Yes" : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <p className="muted">No enrolled recipients yet. Review the shortlist first.</p>
          )}
        </div>
      )}

      {tab === "yaml" && (
        <div className="card stack">
          <p className="muted">
            YAML is the same format the engine sends. The visual editor writes this subset. Variants stay here.
          </p>
          <label>
            Sequence YAML
            <textarea rows={18} value={yaml} onChange={(e) => setYaml(e.target.value)} readOnly={!editable} />
          </label>
          {editable ? (
            <button
              type="button"
              className="secondary"
              disabled={busy || !yaml.trim()}
              onClick={() => void run(() => api.patchCampaign(id, { sequence_yaml: yaml }))}
            >
              Save YAML
            </button>
          ) : null}
        </div>
      )}
    </div>
  );
}
