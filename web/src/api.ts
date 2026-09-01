const BASE = "/api/v1";

export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, body: unknown) {
    const msg =
      typeof body === "object" &&
      body &&
      "error" in body &&
      typeof (body as { error?: { message?: string } }).error?.message === "string"
        ? (body as { error: { message: string } }).error.message
        : `HTTP ${status}`;
    super(msg);
    this.status = status;
    this.body = body;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { raw: text };
    }
  }
  if (!res.ok) throw new ApiError(res.status, data);
  if (
    data &&
    typeof data === "object" &&
    "data" in data &&
    (data as { data: unknown }).data !== undefined
  ) {
    return (data as { data: T }).data;
  }
  return data as T;
}

export type Period = "today" | "7d" | "30d" | "all";

export interface OverviewStats {
  sent: number;
  replies: number;
  reply_rate: number;
  positive_replies?: number;
  bounces: number;
  approx_opens: number;
  range?: string;
  note?: string;
}

export interface SetupStatus {
  workspace_id?: string;
  accounts: number;
  campaigns: number;
  leads: number;
  suppressions?: number;
  encryption_ready: boolean;
  google_oauth_ready?: boolean;
  next_actions?: string[];
}

export interface Suppression {
  id: number;
  kind: string;
  value: string;
  created_at?: string;
}

export interface VerifyResult {
  email: string;
  ok: boolean;
  reason?: string;
  disposable?: boolean;
  mx?: boolean;
}

export interface DNSCheck {
  email: string;
  domain: string;
  mx: boolean;
  mx_records?: string[];
  spf: boolean;
  spf_record?: string;
  dmarc: boolean;
  dmarc_record?: string;
  note?: string;
}

export interface Account {
  id: string | number;
  email: string;
  status: string;
  daily_limit?: number;
  provider?: string;
  workspace_id?: string;
  oauth_health?: string;
  sent_today?: number;
  warmup_status?: string;
  reply_mode?: string;
  domain_verification?: string;
}

export interface Campaign {
  id: string | number;
  name: string;
  status: string;
  workspace_id?: string;
  leads?: number;
  sent?: number;
  replies?: number;
  reply_rate?: number;
  bounces?: number;
  approx_opens?: number;
  next_send?: string;
  created_at?: string;
}

export interface CampaignStats {
  campaign?: string;
  sent?: number;
  replies?: number;
  bounces?: number;
  approx_opens?: number;
  reply_rate?: number;
  steps?: unknown;
  variants?: unknown;
  leads?: unknown;
  note?: string;
}

export interface Lead {
  id: string | number;
  email: string;
  first_name?: string;
  last_name?: string;
  company?: string;
  domain?: string;
  global_status?: string;
}

export interface InboxThread {
  campaign_id: number;
  lead_id: number;
  contact?: string;
  company?: string;
  campaign?: string;
  sender?: string;
  subject?: string;
  latest_message?: string;
  classification?: string;
  timestamp?: string;
}

export interface ThreadMessage {
  id?: number;
  direction?: string;
  from_email?: string;
  subject?: string;
  text_body?: string;
  display_body?: string;
  occurred_at?: string;
}

export const api = {
  overview: (period: Period) => {
    const range = period === "all" ? "" : period;
    const qs = range ? `?range=${range}` : "";
    return request<OverviewStats>(`/overview${qs}`);
  },

  listAccounts: () => request<{ accounts: Account[] }>("/accounts"),

  startGoogleOAuth: () =>
    request<{ authorize_url?: string; status?: string; email?: string }>(
      "/accounts/google/oauth/start",
      { method: "POST", body: "{}" },
    ),

  listCampaigns: () => request<{ campaigns: Campaign[] }>("/campaigns"),

  getCampaign: (id: string | number) => request<Record<string, unknown>>(`/campaigns/${id}`),

  getCampaignStats: (id: string | number) =>
    request<CampaignStats>(`/campaigns/${id}/stats`),

  getCampaignPreview: (id: string | number) =>
    request<unknown>(`/campaigns/${id}/preview?render=1`),

  setup: () => request<SetupStatus>("/setup"),

  createCampaign: (body: {
    name: string;
    sequence_yaml?: string;
    leads_csv?: string;
    accounts?: string[];
    account_ids?: Array<string | number>;
    open_tracking?: boolean;
    draft_only?: boolean;
    send_window_start?: string;
    send_window_end?: string;
    send_days?: string;
    timezone?: string;
  }) =>
    request<{
      campaign_id: number;
      status: string;
      name: string;
      lead_count?: number;
      next_actions?: string[];
    }>("/campaigns", {
      method: "POST",
      body: JSON.stringify({
        name: body.name,
        sequence_yaml: body.sequence_yaml,
        leads_csv: body.leads_csv,
        accounts: body.accounts || [],
        open_tracking: body.open_tracking,
        draft_only: body.draft_only ?? true,
        send_window_start: body.send_window_start,
        send_window_end: body.send_window_end,
        send_days: body.send_days,
        timezone: body.timezone,
      }),
    }),

  cloneCampaign: (id: string | number, body?: { name?: string; leads_csv?: string }) =>
    request<{ campaign_id: number; name: string; status: string }>(`/campaigns/${id}/clone`, {
      method: "POST",
      body: JSON.stringify(body || {}),
    }),

  patchCampaign: (
    id: string | number,
    body: {
      sequence_yaml?: string;
      send_window_start?: string;
      send_window_end?: string;
      send_days?: string;
      timezone?: string;
      open_tracking?: boolean;
    },
  ) =>
    request<unknown>(`/campaigns/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),

  exportCampaignLeads: (id: string | number) =>
    request<{ csv: string; count: number }>(`/campaigns/${id}/leads/export`),

  preflightCampaign: (id: string | number) =>
    request<{ ready?: boolean; warnings?: string[] }>(`/campaigns/${id}/preflight`),

  activateCampaign: (id: string | number) =>
    request<unknown>(`/campaigns/${id}/activate`, {
      method: "POST",
      body: JSON.stringify({ confirm: true }),
    }),

  pauseCampaign: (id: string | number) =>
    request<unknown>(`/campaigns/${id}/pause`, { method: "POST", body: "{}" }),

  resumeCampaign: (id: string | number) =>
    request<unknown>(`/campaigns/${id}/resume`, { method: "POST", body: "{}" }),

  addLeads: (id: string | number, csv: string) =>
    request<unknown>(`/campaigns/${id}/leads`, {
      method: "POST",
      body: JSON.stringify({ csv }),
    }),

  validateLeads: (body: { csv: string }) =>
    request<{
      total: number;
      valid: number;
      invalid: number;
      duplicate: number;
      warnings: string[];
    }>("/leads/validate", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  listLeads: (q?: string) => {
    const qs = q ? `?q=${encodeURIComponent(q)}` : "";
    return request<{ leads: Lead[] }>(`/leads${qs}`);
  },

  exportLeads: (q?: string) => {
    const qs = q ? `?q=${encodeURIComponent(q)}` : "";
    return request<{ csv: string; count: number }>(`/leads/export${qs}`);
  },

  verifyLeads: (body: { emails?: string[]; csv?: string; email?: string }) =>
    request<{ results: VerifyResult[]; valid: number; invalid: number; total: number }>(
      "/leads/verify",
      { method: "POST", body: JSON.stringify(body) },
    ),

  listSuppressions: () => request<{ suppressions: Suppression[] }>("/suppressions"),

  addSuppression: (body: { email?: string; domain?: string; kind?: string; value?: string; csv?: string }) =>
    request<unknown>("/suppressions", { method: "POST", body: JSON.stringify(body) }),

  deleteSuppression: (id: string | number) =>
    request<unknown>(`/suppressions/${id}`, { method: "DELETE" }),

  blacklistLead: (id: string | number) =>
    request<unknown>(`/leads/${id}/blacklist`, { method: "POST", body: "{}" }),

  accountDNS: (id: string | number) => request<DNSCheck>(`/accounts/${id}/dns`),

  removeAccount: (id: string | number) =>
    request<unknown>(`/accounts/${id}/remove`, { method: "POST", body: "{}" }),

  listInbox: () => request<{ threads: InboxThread[] }>("/inbox"),

  getThread: (campaignId: string | number, leadId: string | number) =>
    request<{ messages: ThreadMessage[] }>(`/threads/${campaignId}/${leadId}`),

  replyToThread: (
    campaignId: string | number,
    leadId: string | number,
    body: string,
    confirmTo: string,
    send = false,
  ) =>
    request<unknown>(`/threads/${campaignId}/${leadId}/reply`, {
      method: "POST",
      body: JSON.stringify({ body, confirm_to: confirmTo, send, confirm: send }),
    }),

  suggestReply: (campaignId: string | number, leadId: string | number) =>
    request<{ suggested_body?: string; classification?: string; send_allowed?: boolean }>(
      `/threads/${campaignId}/${leadId}/suggest-reply`,
    ),

  workspace: () =>
    request<{ workspace_id: string }>("/workspace"),

  capabilities: () => request<Capabilities>("/settings/capabilities"),

  listIntegrations: () =>
    request<{ integrations: IntegrationCredential[] }>("/integrations"),

  putIntegration: (body: { provider: string; name: string; secret: string; metadata?: string }) =>
    request<IntegrationCredential>("/integrations", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  deleteIntegration: (id: string | number) =>
    request<{ deleted: boolean }>(`/integrations/${id}`, { method: "DELETE" }),

  testIntegration: (id: string | number) =>
    request<{ ok: boolean; provider?: string; detail?: string }>(`/integrations/${id}/test`, {
      method: "POST",
      body: "{}",
    }),

  startMicrosoftOAuth: () =>
    request<{ authorize_url?: string }>("/accounts/microsoft/oauth/start", {
      method: "POST",
      body: "{}",
    }),

  addSMTPAccount: (body: Record<string, unknown>) =>
    request<Account>("/accounts/smtp", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  addResendAccount: (body: { email: string; api_key: string; daily_limit?: number }) =>
    request<Account>("/accounts/resend", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  addCFEmailAccount: (body: {
    email: string;
    api_token: string;
    account_id: string;
    daily_limit?: number;
  }) =>
    request<Account>("/accounts/cf-email", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  pauseAccount: (id: string | number) =>
    request<unknown>(`/accounts/${id}/pause`, { method: "POST", body: "{}" }),

  resumeAccount: (id: string | number) =>
    request<unknown>(`/accounts/${id}/resume`, { method: "POST", body: "{}" }),

  apolloSearch: (body: {
    q_keywords?: string;
    credential_name?: string;
    per_page?: number;
    person_titles?: string[];
  }) =>
    request<{ leads?: Record<string, string>[]; count?: number; csv?: string }>(
      "/integrations/apollo/search",
      { method: "POST", body: JSON.stringify(body) },
    ),

  sheetsImport: (body: { url: string; campaign_id?: number }) =>
    request<{ preview?: boolean; count?: number; csv?: string; imported?: unknown }>(
      "/integrations/sheets/import",
      { method: "POST", body: JSON.stringify(body) },
    ),
};

export interface Capabilities {
  workspace_id?: string;
  auth_mode?: string;
  mcp_configured?: boolean;
  mcp_endpoint?: string;
  public_base_url?: string;
  sending?: Record<string, boolean>;
  integrations?: Record<string, boolean>;
  encryption_ready?: boolean;
  google_oauth_ready?: boolean;
  microsoft_oauth_ready?: boolean;
}

export interface IntegrationCredential {
  id: number;
  workspace_id?: string;
  provider: string;
  name: string;
  status?: string;
  secret_hint?: string;
  has_secret?: boolean;
}

export function asArray<T>(data: { [k: string]: T[] } | T[], key: string): T[] {
  if (Array.isArray(data)) return data;
  const v = (data as Record<string, T[]>)[key];
  return Array.isArray(v) ? v : [];
}
