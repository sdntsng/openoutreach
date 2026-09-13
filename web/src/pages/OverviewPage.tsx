import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign, type OverviewAction, type OverviewStats, type Period } from "../api";
import { CampaignTable } from "../CampaignTable";
import { PageIntro } from "../ui";
import { useWorkspace } from "../workspace";

const PERIODS: { id: Period; label: string }[] = [
  { id: "today", label: "Today" },
  { id: "7d", label: "7d" },
  { id: "30d", label: "30d" },
  { id: "all", label: "All" },
];

const OPEN_TOOLTIP =
  "Approx. opens are inferred from tracking pixel loads. Image proxies, privacy features, and prefetch can inflate or deflate this number — treat it as directional, not exact.";

function pct(n: number | undefined): string {
  if (n == null || Number.isNaN(n)) return "—";
  const v = n <= 1 ? n * 100 : n;
  return `${v.toFixed(1)}%`;
}

export default function OverviewPage() {
  const ws = useWorkspace();
  const [period, setPeriod] = useState<Period>("7d");
  const [stats, setStats] = useState<OverviewStats | null>(null);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([api.overview(period), api.listCampaigns().catch(() => ({ campaigns: [] as Campaign[] }))])
      .then(([data, camps]) => {
        if (cancelled) return;
        setStats(data);
        setCampaigns(asArray(camps, "campaigns"));
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setStats(null);
          setError(err.message || "Failed to load overview");
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [period]);

  const actions: OverviewAction[] = stats?.actions || [];
  const primary = stats?.primary_action;

  return (
    <div>
      <PageIntro title="Overview">
        Your outbound workspace. Choose a small list, review what they will receive, activate on purpose, and own the
        replies. Connectors live on <Link to="/integrations">Integrations</Link>.
      </PageIntro>

      {primary ? (
        <Link to={primary.to} className="card action-card is-primary">
          <div>
            <div className="connector-name">{countLabel(primary)}</div>
            <p className="muted">{primary.detail}</p>
          </div>
          <span className="badge badge-warn">Next</span>
        </Link>
      ) : (
        <Link to="/campaigns/new" className="card action-card is-primary">
          <div>
            <div className="connector-name">Prepare a small campaign</div>
            <p className="muted">Start with about 20 people. Review the emails, then activate when you are ready.</p>
          </div>
          <span className="badge">Start</span>
        </Link>
      )}

      <div className="motion-grid">
        {actions.map((item) => (
          <Link key={item.key} to={item.to} className={`card action-card ${item.count > 0 ? "" : "is-quiet"}`}>
            <div>
              <div className="connector-name">{countLabel(item)}</div>
              <p className="muted">{item.detail}</p>
            </div>
            <span className="metric-count">{item.count}</span>
          </Link>
        ))}
      </div>

      <p className="muted" style={{ marginTop: "0.25rem" }}>
        {ws.canSend
          ? ws.canReply
            ? "Mailbox can send and receive replies."
            : "A mailbox can send, but reply ingestion is not available on this provider."
          : "No verified mailbox yet — you can still draft and review."}{" "}
        {ws.hasOutbound ? "Outbound webhook is connected." : "Handoffs need an outbound webhook on Integrations."}
      </p>

      <div className="filters" style={{ marginTop: "1.5rem" }}>
        {PERIODS.map((p) => (
          <button
            key={p.id}
            type="button"
            className={period === p.id ? "active" : undefined}
            onClick={() => setPeriod(p.id)}
          >
            {p.label}
          </button>
        ))}
      </div>
      {error && <div className="error">{error}</div>}
      {loading && !stats ? (
        <p className="muted">Loading…</p>
      ) : stats ? (
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
            <div className="value">{pct(stats.reply_rate)}</div>
          </div>
          <div className="metric">
            <div className="label">Positive</div>
            <div className="value">{stats.positive_replies ?? "—"}</div>
          </div>
          <div className="metric">
            <div className="label">Bounces</div>
            <div className="value">{stats.bounces ?? 0}</div>
          </div>
          <div className="metric" title={OPEN_TOOLTIP}>
            <div className="label">Approx. opens</div>
            <div className="value">{stats.approx_opens ?? 0}</div>
          </div>
        </div>
      ) : null}
      <p className="muted" style={{ marginTop: "0.5rem", fontSize: "0.85rem" }}>
        Hover <strong>Approx. opens</strong> for notes on image-proxy noise.
      </p>
      <h2>Campaigns</h2>
      <CampaignTable campaigns={campaigns} empty="No campaigns yet. Prepare a draft — activate is a separate step." />
    </div>
  );
}

function countLabel(item: OverviewAction): string {
  const n = item.count;
  switch (item.key) {
    case "shortlist":
      return `${n} lead${n === 1 ? "" : "s"} awaiting review`;
    case "drafts":
      return `${n} draft${n === 1 ? "" : "s"} ready`;
    case "replies":
      return `${n} ${n === 1 ? "reply needs" : "replies need"} attention`;
    case "handoff":
      return `${n} handoff${n === 1 ? "" : "s"} failed`;
    default:
      return `${n} ${item.label}`;
  }
}
