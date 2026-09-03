import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type CampaignStats } from "../api";
import { LeadImport } from "../LeadImport";
import { StatusChip } from "../ui";

type Tab = "campaign" | "leads";

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
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [tab, setTab] = useState<Tab>("campaign");
  const [campaign, setCampaign] = useState<Record<string, unknown> | null>(null);
  const [stats, setStats] = useState<CampaignStats | null>(null);
  const [preview, setPreview] = useState<unknown>(null);
  const [sequence, setSequence] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function reload() {
    const c = await api.getCampaign(id);
    setCampaign(c);
    const yaml = str(c.sequence_yaml || c.sequence, "");
    setSequence(yaml === "—" ? "" : yaml);
    try {
      setStats(await api.getCampaignStats(id));
    } catch {
      setStats(null);
    }
  }

  useEffect(() => {
    setError(null);
    reload().catch((err: Error) => setError(err.message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await action();
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const status = str(campaign?.status, "");
  const name = str(campaign?.name, "Campaign");
  const leadCount = Number(campaign?.leads ?? (Array.isArray(stats?.leads) ? stats?.leads.length : 0));
  const editable = status === "draft" || status === "paused";

  if (!campaign && !error) return <p className="muted">Loading…</p>;

  return (
    <div>
      <div className="row-actions" style={{ marginBottom: "0.5rem" }}>
        <Link to="/campaigns">← Campaigns</Link>
      </div>
      <div className="row-actions" style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
        <div>
          <h1 style={{ margin: 0 }}>{name}</h1>
          <p className="status-banner">
            <StatusChip status={status} />{" "}
            {status === "paused"
              ? "Sending stopped. Replies are still monitored."
              : status === "draft"
                ? "Draft — nothing sends until you activate with confirm."
                : status === "active"
                  ? "Active — tick sends due mail from scheduled_sends."
                  : null}
          </p>
        </div>
        <div className="row-actions">
          {status === "draft" && (
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                if (window.confirm("Activate this campaign? This will start sending scheduled emails.")) {
                  void run(() => api.activateCampaign(id));
                }
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
            <button type="button" disabled={busy} onClick={() => void run(() => api.resumeCampaign(id))}>
              Resume
            </button>
          )}
          {editable && (
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={() => {
                void run(() =>
                  api.preflightCampaign(id).then((res) => {
                    const warns = res.warnings?.length ? res.warnings.join(" · ") : "no warnings";
                    window.alert(`${res.ready ? "Ready" : "Not ready"} — ${warns}`);
                    return res;
                  }),
                );
              }}
            >
              Preflight
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

      <div className="tabs">
        <button type="button" className={tab === "campaign" ? "active" : undefined} onClick={() => setTab("campaign")}>
          Campaign
        </button>
        <button type="button" className={tab === "leads" ? "active" : undefined} onClick={() => setTab("leads")}>
          Leads{leadCount ? ` (${leadCount})` : ""}
        </button>
      </div>

      {tab === "campaign" && campaign && (
        <div className="stack">
          {stats && (
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
          )}
          <div className="card stack">
            <p className="muted">
              Send window {str(campaign.send_window)} · {str(campaign.timezone)} · accounts {str(campaign.accounts)}
            </p>
            <label>
              Sequence YAML
              <textarea
                rows={16}
                value={sequence}
                onChange={(e) => setSequence(e.target.value)}
                readOnly={!editable}
              />
            </label>
            {editable ? (
              <button
                type="button"
                className="secondary"
                disabled={busy || !sequence.trim()}
                onClick={() => void run(() => api.patchCampaign(id, { sequence_yaml: sequence }))}
              >
                Save sequence
              </button>
            ) : (
              <p className="muted">Pause the campaign to edit the sequence.</p>
            )}
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={() => {
                setBusy(true);
                api
                  .getCampaignPreview(id)
                  .then(setPreview)
                  .catch((err: Error) => setError(err.message))
                  .finally(() => setBusy(false));
              }}
            >
              Load preview
            </button>
            {preview ? <pre className="code">{JSON.stringify(preview, null, 2)}</pre> : null}
          </div>
        </div>
      )}

      {tab === "leads" && (
        <div className="stack">
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
              Export leads
            </button>
          </div>
          <LeadImport campaignId={id} onImported={() => void reload()} />
          {Array.isArray(stats?.leads) && stats.leads.length > 0 ? (
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
            <p className="muted">No leads on this campaign yet.</p>
          )}
        </div>
      )}
    </div>
  );
}
