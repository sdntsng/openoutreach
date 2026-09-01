import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type CampaignStats } from "../api";

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
  const [apolloQ, setApolloQ] = useState("");
  const [apolloTitles, setApolloTitles] = useState("");
  const [apolloRows, setApolloRows] = useState<Record<string, string>[]>([]);
  const [sheetURL, setSheetURL] = useState("");

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
          <div className="form-grid" style={{ marginTop: "1.25rem" }}>
            <h3 style={{ margin: 0 }}>Import leads (draft-safe)</h3>
            <label>
              Apollo search
              <input
                value={apolloQ}
                onChange={(e) => setApolloQ(e.target.value)}
                placeholder="keywords, company…"
              />
            </label>
            <label>
              Titles (optional)
              <input
                value={apolloTitles}
                onChange={(e) => setApolloTitles(e.target.value)}
                placeholder="CRO, CMO"
              />
            </label>
            <button
              type="button"
              disabled={busy || !apolloQ.trim()}
              onClick={() => {
                setBusy(true);
                setError(null);
                const titles = apolloTitles
                  .split(",")
                  .map((s) => s.trim())
                  .filter(Boolean);
                api
                  .apolloSearch({
                    q_keywords: apolloQ.trim(),
                    per_page: 10,
                    person_titles: titles.length ? titles : undefined,
                  })
                  .then((res) => setApolloRows(res.leads || []))
                  .catch((err: Error) => setError(err.message))
                  .finally(() => setBusy(false));
              }}
            >
              Search Apollo
            </button>
            {apolloRows.length > 0 && (
              <>
                <p className="muted">{apolloRows.length} preview rows (not imported yet)</p>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    const csv = [
                      "email,first_name,last_name,company,domain,title,linkedin_url",
                      ...apolloRows.map((r) =>
                        [r.email, r.first_name, r.last_name, r.company, r.domain, r.title, r.linkedin_url]
                          .map((v) => `"${String(v || "").replaceAll('"', '""')}"`)
                          .join(","),
                      ),
                    ].join("\n");
                    void run(() => api.addLeads(id, csv));
                  }}
                >
                  Import Apollo preview
                </button>
              </>
            )}
            <label>
              Google Sheet or CSV URL
              <input
                value={sheetURL}
                onChange={(e) => setSheetURL(e.target.value)}
                placeholder="https://docs.google.com/spreadsheets/d/…"
              />
            </label>
            <button
              type="button"
              disabled={busy || !sheetURL.trim()}
              onClick={() => {
                const cid = Number(campaign.id);
                void run(() => api.sheetsImport({ url: sheetURL.trim(), campaign_id: cid }));
              }}
            >
              Import from Sheet
            </button>
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
