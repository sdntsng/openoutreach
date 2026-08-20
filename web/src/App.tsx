import { NavLink, Route, Routes } from "react-router-dom";
import OverviewPage from "./pages/OverviewPage";
import CampaignsPage from "./pages/CampaignsPage";
import CampaignDetailPage from "./pages/CampaignDetailPage";
import CampaignCreatePage from "./pages/CampaignCreatePage";
import InboxPage from "./pages/InboxPage";
import LeadsPage from "./pages/LeadsPage";
import AccountsPage from "./pages/AccountsPage";
import SettingsPage from "./pages/SettingsPage";

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

export default function App() {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">OpenOutreach</div>
        <NavGroup label="Main" items={NAV_MAIN} />
        <NavGroup label="Mailbox" items={NAV_MAIL} />
        <NavGroup label="Setup" items={NAV_SETUP} />
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
