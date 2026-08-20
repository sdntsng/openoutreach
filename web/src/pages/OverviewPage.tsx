import { useEffect, useState } from "react";
import { api, type OverviewStats, type Period } from "../api";

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
  const [period, setPeriod] = useState<Period>("7d");
  const [stats, setStats] = useState<OverviewStats | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    api
      .overview(period)
      .then((data) => {
        if (!cancelled) setStats(data);
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

  return (
    <div>
      <h1>Overview</h1>
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
    </div>
  );
}
