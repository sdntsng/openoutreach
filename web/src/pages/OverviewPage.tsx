import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign, type OverviewStats, type Period, type SetupStatus } from "../api";
import { CampaignTable } from "../CampaignTable";

const PERIODS: { id: Period; label: string }[] = [
  { id: "today", label: "Today" },
  { id: "7d", label: "7d" },
  { id: "30d", label: "30d" },
  { id: "all", label: "All" },
];

const OPEN_TOOLTIP =
  "Approx. opens are inferred from tracking pixel loads. Image proxies, privacy features, and prefetch can inflate or deflate this number — treat it as directional, not exact.";

const SETUP_STEPS: { action: string; label: string; to: string; done: (s: SetupStatus) => boolean }[] = [
  { action: "connect_account", label: "Connect a sending account", to: "/integrations?kind=send", done: (s) => s.accounts > 0 },
  { action: "import_leads", label: "Import leads (CSV or a connector)", to: "/leads", done: (s) => s.leads > 0 },
  { action: "create_draft", label: "Create a draft campaign", to: "/campaigns/new", done: (s) => s.campaigns > 0 },
  {
    action: "preview_and_activate",
    label: "Preview, then activate with confirm",
    to: "/campaigns",
    done: (s) => s.accounts > 0 && s.leads > 0 && s.campaigns > 0,
  },
];

function pct(n: number | undefined): string {
  if (n == null || Number.isNaN(n)) return "—";
  const v = n <= 1 ? n * 100 : n;
  return `${v.toFixed(1)}%`;
}

export default function OverviewPage() {
  const [period, setPeriod] = useState<Period>("7d");
  const [stats, setStats] = useState<OverviewStats | null>(null);
  const [setup, setSetup] = useState<SetupStatus | null>(null);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([
      api.overview(period),
      api.setup().catch(() => null),
      api.listCampaigns().catch(() => ({ campaigns: [] as Campaign[] })),
    ])
      .then(([data, s, camps]) => {
        if (cancelled) return;
        setStats(data);
        setSetup(s);
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

  const showChecklist = setup && (setup.accounts === 0 || setup.campaigns === 0 || setup.leads === 0);

  return (
    <div>
      <h1>Overview</h1>
      {showChecklist && setup && (
        <div className="panel" style={{ marginBottom: "1.25rem" }}>
          <h2 style={{ marginTop: 0 }}>Get started</h2>
          <p className="muted">Connect a mailbox, import leads, then create a draft. Activate is a separate confirm.</p>
          <ol style={{ margin: "0.75rem 0 0", paddingLeft: "1.2rem" }}>
            {SETUP_STEPS.map((step) => {
              const done = step.done(setup);
              return (
                <li key={step.action} style={{ marginBottom: "0.4rem" }}>
                  {done ? (
                    <span className="muted">{step.label} — done</span>
                  ) : (
                    <Link to={step.to}>{step.label}</Link>
                  )}
                </li>
              );
            })}
          </ol>
          {!setup.encryption_ready && (
            <p className="muted" style={{ marginTop: "0.75rem" }}>
              Vault is not ready. Set <code>CREDENTIAL_ENCRYPTION_KEY</code> once on the server — no extra provider flags.
            </p>
          )}
        </div>
      )}
      <div className="filters">
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
