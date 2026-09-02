import type { Account, Capabilities, IntegrationCredential } from "./api";

export type ConnectorKind = "send" | "leads" | "events";
export type ConnectMode = "oauth" | "account" | "vault" | "file" | "webhook";

export interface Connector {
  id: string;
  name: string;
  mark: string;
  color: string;
  kind: ConnectorKind;
  mode: ConnectMode;
  blurb: string;
  capGroup: "sending" | "integrations" | "always";
  capKey?: string;
  accountProviders?: string[];
  vaultProvider?: string;
}

export const CONNECTORS: Connector[] = [
  {
    id: "gmail",
    name: "Google",
    mark: "G",
    color: "#4285F4",
    kind: "send",
    mode: "oauth",
    blurb: "Gmail / Workspace mailbox. OAuth; replies stay in-thread.",
    capGroup: "sending",
    capKey: "gmail",
    accountProviders: ["google", "gmail"],
  },
  {
    id: "microsoft",
    name: "Microsoft 365",
    mark: "M",
    color: "#5B5FC7",
    kind: "send",
    mode: "oauth",
    blurb: "Outlook / Microsoft 365. Graph send + read.",
    capGroup: "sending",
    capKey: "microsoft",
    accountProviders: ["microsoft", "outlook"],
  },
  {
    id: "smtp",
    name: "SMTP / IMAP",
    mark: "S",
    color: "#3d9b84",
    kind: "send",
    mode: "account",
    blurb: "Any mailbox host. Password is vaulted; never shown again.",
    capGroup: "sending",
    capKey: "smtp_imap",
    accountProviders: ["smtp", "smtp_imap"],
  },
  {
    id: "resend",
    name: "Resend",
    mark: "R",
    color: "#111111",
    kind: "send",
    mode: "account",
    blurb: "Send-only API mailer. Weak for cold-inbox reputation.",
    capGroup: "sending",
    capKey: "resend",
    accountProviders: ["resend"],
  },
  {
    id: "cf_email",
    name: "Cloudflare Email",
    mark: "C",
    color: "#F6821F",
    kind: "send",
    mode: "account",
    blurb: "Transactional Email Service. Replies via Email Routing.",
    capGroup: "sending",
    capKey: "cf_email",
    accountProviders: ["cf_email", "cloudflare"],
  },
  {
    id: "ses",
    name: "Amazon SES",
    mark: "A",
    color: "#FF9900",
    kind: "send",
    mode: "account",
    blurb: "SES SMTP endpoint. Same vaulted SMTP path as a mailbox.",
    capGroup: "sending",
    capKey: "ses",
    accountProviders: ["ses", "smtp", "smtp_imap"],
  },
  {
    id: "csv",
    name: "CSV file",
    mark: "CSV",
    color: "#3d9b84",
    kind: "leads",
    mode: "file",
    blurb: "Upload or paste email, first_name, company.",
    capGroup: "always",
  },
  {
    id: "sheets",
    name: "Google Sheets",
    mark: "Sh",
    color: "#0F9D58",
    kind: "leads",
    mode: "vault",
    blurb: "Import a published sheet or CSV URL into a campaign.",
    capGroup: "integrations",
    capKey: "sheets",
    vaultProvider: "sheets",
  },
  {
    id: "apollo",
    name: "Apollo",
    mark: "Ap",
    color: "#7B61FF",
    kind: "leads",
    mode: "vault",
    blurb: "People search. Preview, then import into a draft campaign.",
    capGroup: "integrations",
    capKey: "apollo",
    vaultProvider: "apollo",
  },
  {
    id: "clay",
    name: "Clay",
    mark: "Cl",
    color: "#6C47FF",
    kind: "leads",
    mode: "webhook",
    blurb: "Signed HTTP ingest. HMAC in the vault; never activate on ingest.",
    capGroup: "integrations",
    capKey: "clay",
    vaultProvider: "clay",
  },
  {
    id: "webhook",
    name: "Generic webhook",
    mark: "WH",
    color: "#1c2421",
    kind: "leads",
    mode: "webhook",
    blurb: "Any enricher → POST /integrations/generic/ingest.",
    capGroup: "integrations",
    capKey: "webhook",
    vaultProvider: "webhook",
  },
  {
    id: "hunter",
    name: "Hunter",
    mark: "H",
    color: "#FA5D00",
    kind: "leads",
    mode: "vault",
    blurb: "Email finder key. Stored encrypted; used from MCP/API.",
    capGroup: "integrations",
    capKey: "hunter",
    vaultProvider: "hunter",
  },
  {
    id: "outbound",
    name: "Outbound webhook",
    mark: "Out",
    color: "#4A90D9",
    kind: "events",
    mode: "vault",
    blurb: "POST sent / reply / bounce after each tick. Failures never block send.",
    capGroup: "integrations",
    capKey: "outbound",
    vaultProvider: "outbound",
  },
  {
    id: "warmup",
    name: "Warmup status",
    mark: "W",
    color: "#C47B3A",
    kind: "events",
    mode: "vault",
    blurb: "Optional health badge only. Warmup traffic never enters Tick.",
    capGroup: "integrations",
    capKey: "warmup",
    vaultProvider: "warmup",
  },
];

export function connectorEnabled(c: Connector, caps: Capabilities | null): boolean {
  if (c.capGroup === "always") return true;
  if (!caps) return true;
  if (!c.capKey) return true;
  const map = c.capGroup === "sending" ? caps.sending : caps.integrations;
  if (!map || !(c.capKey in map)) return true;
  return Boolean(map[c.capKey]);
}

export function connectorConnected(
  c: Connector,
  accounts: Account[],
  creds: IntegrationCredential[],
): boolean {
  if (c.mode === "file") return true;
  if (c.accountProviders?.length) {
    const hit = accounts.some((a) =>
      c.accountProviders!.includes((a.provider || "").toLowerCase()),
    );
    if (c.id === "ses") {
      return accounts.some((a) => (a.provider || "").toLowerCase() === "ses");
    }
    if (hit) return true;
  }
  if (c.vaultProvider) {
    return creds.some((row) => row.provider === c.vaultProvider && row.has_secret !== false);
  }
  return false;
}

export const TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Sao_Paulo",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "Asia/Kolkata",
  "Asia/Singapore",
  "Asia/Tokyo",
  "Australia/Sydney",
];

export const HOURS = Array.from({ length: 24 }, (_, h) => {
  const hh = String(h).padStart(2, "0");
  return [`${hh}:00`, `${hh}:30`] as const;
}).flat();
