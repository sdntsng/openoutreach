import { NavLink, Navigate, Route, Routes, useLocation, useSearchParams } from "react-router-dom";
import { useEffect, useState, type ReactNode } from "react";
import { safeRedirect, signOut, useAuth } from "./auth-client";
import { api, asArray, type Campaign, type InboxCounts } from "./api";
import OverviewPage from "./pages/OverviewPage";
import CampaignsPage from "./pages/CampaignsPage";
import CampaignDetailPage from "./pages/CampaignDetailPage";
import CampaignCreatePage from "./pages/CampaignCreatePage";
import InboxPage from "./pages/InboxPage";
import LeadsPage from "./pages/LeadsPage";
import AccountsPage from "./pages/AccountsPage";
import SettingsPage from "./pages/SettingsPage";
import IntegrationsPage from "./pages/IntegrationsPage";
import SuppressionsPage from "./pages/SuppressionsPage";
import ProjectPage from "./pages/ProjectPage";
import TemplatesPage from "./pages/TemplatesPage";
import SchedulePage from "./pages/SchedulePage";
import TargetingPage from "./pages/TargetingPage";
import SignInPage from "./pages/SignInPage";
import SignUpPage from "./pages/SignUpPage";

function NavGroup({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="nav-section">
      <div className="nav-label">{label}</div>
      <nav className="nav">{children}</nav>
    </div>
  );
}

function GuestOnly({ children }: { children: ReactNode }) {
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
  const location = useLocation();
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [inboxCounts, setInboxCounts] = useState<InboxCounts | null>(null);

  useEffect(() => {
    api
      .listCampaigns()
      .then((data) => setCampaigns(asArray(data, "campaigns")))
      .catch(() => setCampaigns([]));
    api
      .listInbox("needs")
      .then((d) => setInboxCounts(d.counts || null))
      .catch(() => setInboxCounts(null));
  }, [location.pathname]);

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
  const needs = inboxCounts?.needs ?? 0;

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">OpenOutreach</div>
        <NavGroup label="Main">
          <NavLink to="/" end className={({ isActive }) => (isActive ? "active" : undefined)}>
            Overview
          </NavLink>
        </NavGroup>
        <NavGroup label="Mailbox">
          <NavLink
            to="/inbox"
            end
            className={({ isActive }) => {
              const sent = new URLSearchParams(location.search).get("box") === "sent";
              return isActive && !sent ? "active" : undefined;
            }}
          >
            Inbox
            {needs > 0 ? <span className="nav-badge">{needs}</span> : null}
          </NavLink>
          <NavLink
            to="/inbox?box=sent"
            className={() =>
              location.pathname === "/inbox" && new URLSearchParams(location.search).get("box") === "sent"
                ? "active"
                : undefined
            }
          >
            Sent
          </NavLink>
        </NavGroup>
        <NavGroup label="Setup">
          <NavLink to="/project" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Project
          </NavLink>
          <NavLink to="/templates" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Email templates
          </NavLink>
          <NavLink to="/schedule" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Sending schedule
          </NavLink>
          <NavLink to="/targeting" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Targeting
          </NavLink>
          <NavLink to="/integrations" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Integrations
          </NavLink>
          <NavLink to="/suppressions" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Suppress list
          </NavLink>
          <NavLink to="/leads" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Leads
          </NavLink>
          <NavLink to="/accounts" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Sending Accounts
          </NavLink>
        </NavGroup>
        <NavGroup label="Campaigns">
          {campaigns.slice(0, 8).map((c) => (
            <NavLink
              key={String(c.id)}
              to={`/campaigns/${c.id}`}
              className={({ isActive }) => (isActive ? "active" : undefined)}
            >
              <span className={`nav-dot status-${(c.status || "").toLowerCase()}`} />
              {c.name}
            </NavLink>
          ))}
          <NavLink to="/campaigns/new" className={({ isActive }) => (isActive ? "active nav-cta" : "nav-cta")}>
            + New campaign
          </NavLink>
          <NavLink to="/campaigns" end className={({ isActive }) => (isActive ? "active" : undefined)}>
            All campaigns
          </NavLink>
        </NavGroup>
        <div className="sidebar-foot">
          {showUser ? (
            <div className="sidebar-user" title={auth.user?.email}>
              {auth.user?.name || auth.user?.email}
            </div>
          ) : null}
          <NavLink to="/settings" className={({ isActive }) => (isActive ? "active" : undefined)}>
            Settings
          </NavLink>
          {showUser ? (
            <button type="button" className="secondary" onClick={() => signOut(auth.mode)}>
              Sign out
            </button>
          ) : null}
        </div>
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
          <Route path="/integrations" element={<IntegrationsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/suppressions" element={<SuppressionsPage />} />
          <Route path="/project" element={<ProjectPage />} />
          <Route path="/templates" element={<TemplatesPage />} />
          <Route path="/schedule" element={<SchedulePage />} />
          <Route path="/targeting" element={<TargetingPage />} />
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
