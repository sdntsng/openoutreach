import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type CampaignStats } from "../api";
import { LeadImport } from "../LeadImport";

type Tab = "overview" | "sequence" | "preview" | "stats";

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
  const [tab, setTab] = useState<Tab>("overview");
  const [campaign, setCampaign] = useState<Record<string, unknown> | null>(null);
  const [stats, setStats] = useState<CampaignStats | null>(null);
  const [preview, setPreview] = useState<unknown>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function reload() {
    const c = await api.getCampaign(id);
    setCampaign(c);
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

  useEffect(() => {
    if (tab !== "preview") return;
    api
      .getCampaignPreview(id)
      .then(setPreview)
      .catch((err: Error) => setError(err.message));
  }, [tab, id]);

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

  if (!campaign && !error) return <p className="muted">Loading…</p>;

  return (
    <div>
      <div className="row-actions" style={{ marginBottom: "0.5rem" }}>
        <Link to="/campaigns">← Campaigns</Link>
      </div>
      <div className="row-actions" style={{ justifyContent: "space-between" }}>
        <h1 style={{ margin: 0 }}>{name}</h1>
        <div className="row-actions">
          {status === "draft" && (
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                if (
                  window.confirm(
                    "Activate this campaign? This will start sending scheduled emails.",
                  )
                ) {
                  void run(() => api.activateCampaign(id));
                }
              }}
            >
              Activate
            </button>
          )}
          {status === "active" && (
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={() => void run(() => api.pauseCampaign(id))}
            >
              Pause
            </button>
          )}
          {status === "paused" && (
            <button
              type="button"
              disabled={busy}
              onClick={() => void run(() => api.resumeCampaign(id))}
            >
              Resume
            </button>
          )}
          {(status === "draft" || status === "paused") && (
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
      </div>
      {campaign && (
        <p className="muted">
          Status: <strong>{status}</strong>
        </p>
      )}
      {error && <div className="error">{error}</div>}

      <div className="tabs">
        {(["overview", "sequence", "preview", "stats"] as Tab[]).map((t) => (
          <button
            key={t}
            type="button"
            className={tab === t ? "active" : undefined}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === "overview" && campaign && (
        <div className="panel">
          <p>
            <strong>Leads:</strong> {str(campaign.leads)}
          </p>
          <p>
            <strong>Accounts:</strong> {str(campaign.accounts)}
          </p>
          <p>
            <strong>Send window:</strong> {str(campaign.send_window)} ({str(campaign.timezone)})
          </p>
          <div style={{ marginTop: "1.25rem" }}>
            <LeadImport campaignId={id} onImported={() => void reload()} />
          </div>
          {stats && (
            <div className="metrics" style={{ marginTop: "1rem" }}>
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
        </div>
      )}

      {tab === "sequence" && (
        <pre
          className="panel"
          style={{ whiteSpace: "pre-wrap", fontSize: "12.5px", overflow: "auto" }}
        >
          {str(campaign?.sequence, "(no sequence stored)")}
        </pre>
      )}

      {tab === "preview" && (
        <pre
          className="panel"
          style={{ whiteSpace: "pre-wrap", fontSize: "12px", overflow: "auto", maxHeight: 480 }}
        >
          {JSON.stringify(preview, null, 2)}
        </pre>
      )}

      {tab === "stats" && stats && (
        <table>
          <tbody>
            {Object.entries(stats).map(([k, v]) => (
              <tr key={k}>
                <td>{k === "approx_opens" ? "Approx. opens" : k}</td>
                <td>{typeof v === "object" ? JSON.stringify(v) : String(v)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
