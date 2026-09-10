import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign, type OverviewStats, type Period } from "../api";
import { CampaignTable } from "../CampaignTable";
import { PageIntro, StatusBadge } from "../ui";
import { GATES, gateReady, useWorkspace, type GateId } from "../workspace";

const PERIODS: { id: Period; label: string }[] = [
  { id: "today", label: "Today" },
  { id: "7d", label: "7d" },
  { id: "30d", label: "30d" },
  { id: "all", label: "All" },
];

const OPEN_TOOLTIP =
  "Approx. opens are inferred from tracking pixel loads. Image proxies, privacy features, and prefetch can inflate or deflate this number — treat it as directional, not exact.";

type CapStatus = "ready" | "connect" | "soon";

interface Capability {
  title: string;
  blurb: string;
  to: string;
  status: CapStatus;
  gate?: GateId;
}

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

  const groups: { label: string; items: Capability[] }[] = [
    {
      label: "Find",
      items: [
        {
          title: "CSV import",
          blurb: "Upload or paste. Always on — no vendor key.",
          to: "/leads",
          status: "ready",
        },
        {
          title: "Apollo",
          blurb: "People search. Preview, then import into a draft.",
          to: gateReady(ws, "apollo") ? "/leads" : GATES.apollo.to,
          status: gateReady(ws, "apollo") ? "ready" : "connect",
          gate: "apollo",
        },
        {
          title: "Google Sheets",
          blurb: "Published sheet or CSV URL into a campaign.",
          to: gateReady(ws, "sheets") ? "/leads" : GATES.sheets.to,
          status: gateReady(ws, "sheets") ? "ready" : "connect",
          gate: "sheets",
        },
        {
          title: "Clay / webhook",
          blurb: "Signed ingest. Never activates on arrival.",
          to: gateReady(ws, "clay") ? "/leads" : GATES.clay.to,
          status: gateReady(ws, "clay") ? "ready" : "connect",
          gate: "clay",
        },
      ],
    },
    {
      label: "Reach",
      items: [
        {
          title: "Sequences",
          blurb: "Draft YAML, then activate with confirm.",
          to: gateReady(ws, "sender") ? "/campaigns/new" : GATES.sender.to,
          status: gateReady(ws, "sender") ? "ready" : "connect",
          gate: "sender",
        },
        {
          title: "Sending accounts",
          blurb: "Your Gmail, Microsoft 365, or SMTP — not a rented pool.",
          to: "/integrations?kind=send",
          status: gateReady(ws, "sender") ? "ready" : "connect",
          gate: "sender",
        },
        {
          title: "Inbox warming",
          blurb: "Badge only. Warmup traffic never enters Tick.",
          to: "/integrations?connect=warmup",
          status: "soon",
        },
      ],
    },
    {
      label: "Reply",
      items: [
        {
          title: "Unified inbox",
          blurb: "Needs reply, got reply, sent — same Gmail thread.",
          to: gateReady(ws, "sender") ? "/inbox" : GATES.sender.to,
          status: gateReady(ws, "sender") ? "ready" : "connect",
          gate: "sender",
        },
        {
          title: "Suggested replies",
          blurb: "Uses project facts. You still send.",
          to: gateReady(ws, "sender") ? "/inbox" : GATES.sender.to,
          status: gateReady(ws, "sender") ? "ready" : "connect",
          gate: "sender",
        },
      ],
    },
    {
      label: "Route",
      items: [
        {
          title: "Outbound webhook",
          blurb: "POST sent / reply / bounce. Failures never block send.",
          to: GATES.outbound.to,
          status: gateReady(ws, "outbound") ? "ready" : "connect",
          gate: "outbound",
        },
        {
          title: "MCP / agents",
          blurb: "Same engine as the dashboard. Create ≠ send.",
          to: gateReady(ws, "mcp") ? "/settings" : GATES.mcp.to,
          status: gateReady(ws, "mcp") ? "ready" : "connect",
          gate: "mcp",
        },
        {
          title: "LinkedIn steps",
          blurb: "Webhook ingest only. We do not scrape Sales Nav.",
          to: "/integrations",
          status: "soon",
        },
      ],
    },
  ];

  return (
    <div>
      <PageIntro title="Command center">
        One motion: find leads, reach from your mailbox, reply in-thread, route the hot ones. A
        capability stays visible when the first integration is missing — connect that, then use it.
      </PageIntro>

      <div className="motion-grid">
        {groups.map((g) => (
          <section key={g.label} className="card motion-col">
            <div className="nav-label">{g.label}</div>
            {g.items.map((item) => (
              <Link
                key={item.title}
                to={item.to}
                className={`motion-item ${item.status === "ready" ? "" : "is-gated"}`}
              >
                <div>
                  <div className="connector-name">{item.title}</div>
                  <p className="muted">{item.blurb}</p>
                </div>
                {item.status === "ready" ? (
                  <StatusBadge ok on="Ready" />
                ) : item.status === "soon" ? (
                  <span className="badge">Soon</span>
                ) : (
                  <span className="badge badge-warn">Connect</span>
                )}
              </Link>
            ))}
          </section>
        ))}
      </div>

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
      <CampaignTable campaigns={campaigns} />
    </div>
  );
}
