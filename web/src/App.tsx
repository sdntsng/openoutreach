import { NavLink, Navigate, Route, Routes, useSearchParams } from "react-router-dom";
import { safeRedirect, signOut, useAuth } from "./auth-client";
import OverviewPage from "./pages/OverviewPage";
import CampaignsPage from "./pages/CampaignsPage";
import CampaignDetailPage from "./pages/CampaignDetailPage";
import CampaignCreatePage from "./pages/CampaignCreatePage";
import InboxPage from "./pages/InboxPage";
import LeadsPage from "./pages/LeadsPage";
import AccountsPage from "./pages/AccountsPage";
import SettingsPage from "./pages/SettingsPage";
import SignInPage from "./pages/SignInPage";
import SignUpPage from "./pages/SignUpPage";

const NAV_MAIN = [{ to: "/", label: "Overview", end: true as const }];
const NAV_MAIL = [
  { to: "/inbox", label: "Inbox" },
  { to: "/campaigns", label: "Campaigns" },
];
const NAV_SETUP = [
  { to: "/leads", label: "Leads" },
  { to: "/accounts", label: "Sending Accounts" },
  { to: "/settings", label: "Settings" },
];

function NavGroup({
  label,
  items,
}: {
  label: string;
  items: Array<{ to: string; label: string; end?: boolean }>;
}) {
  return (
    <div className="nav-section">
      <div className="nav-label">{label}</div>
      <nav className="nav">
        {items.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) => (isActive ? "active" : undefined)}
          >
            {item.label}
          </NavLink>
        ))}
      </nav>
    </div>
  );
}

function GuestOnly({ children }: { children: React.ReactNode }) {
  const auth = useAuth();
  const [params] = useSearchParams();
  if (auth.pending) {
    return (
      <div className="auth-shell">
        <p className="muted">Loading…</p>
      </div>
    );
  }
  if (auth.mode !== "hosted") {
    return <Navigate to="/" replace />;
  }
  if (auth.user) {
    return <Navigate to={safeRedirect(params.get("redirect"))} replace />;
  }
  return children;
}

function AuthedShell() {
  const auth = useAuth();
  if (auth.pending) {
    return (
      <div className="auth-shell">
        <p className="muted">Loading…</p>
      </div>
    );
  }
  if (auth.mode === "hosted" && !auth.user) {
    const next = window.location.pathname + window.location.search;
    return <Navigate to={`/sign-in?redirect=${encodeURIComponent(next)}`} replace />;
  }
  if (auth.mode === "cloudflare_access" && auth.accessConfigured === false) {
    return (
      <div className="auth-shell">
        <div className="auth-card">
          <h1>Cloudflare Access is not configured</h1>
          <p className="muted">
            Set CF_ACCESS_AUD to your Access application AUD tag, or switch AUTH_MODE to hosted
            (in-app Google/email) or local_noauth.
          </p>
        </div>
      </div>
    );
  }

  const showUser = Boolean(auth.user && auth.mode !== "local_noauth");

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">OpenOutreach</div>
        <NavGroup label="Main" items={NAV_MAIN} />
        <NavGroup label="Mailbox" items={NAV_MAIL} />
        <NavGroup label="Setup" items={NAV_SETUP} />
        {showUser ? (
          <div className="sidebar-foot">
            <div className="sidebar-user" title={auth.user?.email}>
              {auth.user?.name || auth.user?.email}
            </div>
            <button type="button" className="secondary" onClick={() => signOut(auth.mode)}>
              Sign out
            </button>
          </div>
        ) : null}
      </aside>
      <main className="main">
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/campaigns" element={<CampaignsPage />} />
          <Route path="/campaigns/new" element={<CampaignCreatePage />} />
          <Route path="/campaigns/:id" element={<CampaignDetailPage />} />
          <Route path="/inbox" element={<InboxPage />} />
          <Route path="/leads" element={<LeadsPage />} />
          <Route path="/accounts" element={<AccountsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route
        path="/sign-in"
        element={
          <GuestOnly>
            <SignInPage />
          </GuestOnly>
        }
      />
      <Route
        path="/sign-up"
        element={
          <GuestOnly>
            <SignUpPage />
          </GuestOnly>
        }
      />
      <Route path="*" element={<AuthedShell />} />
    </Routes>
  );
}
