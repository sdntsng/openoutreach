/**
 * MCP JSON-RPC over HTTP for OpenOutreach.
 * Supports tools/list + tools/call (Cursor-compatible Streamable HTTP / simple JSON-RPC).
 * Never returns OAuth or credential tokens.
 */

export interface McpEnv {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  OUTREACH: DurableObjectNamespace<any>;
  MCP_BEARER_TOKEN?: string;
  INTERNAL_CONTAINER_TOKEN?: string;
}

type JsonRpcId = string | number | null;

interface JsonRpcRequest {
  jsonrpc?: string;
  id?: JsonRpcId;
  method?: string;
  params?: Record<string, unknown>;
}

interface ToolDef {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  /** Map tool call → container API request */
  call: (args: Record<string, unknown>) => {
    method: string;
    path: string;
    body?: unknown;
  };
}

const TOOLS: ToolDef[] = [
  {
    name: "outreach_list_accounts",
    description: "List sending accounts in the current workspace.",
    inputSchema: {
      type: "object",
      properties: {},
      additionalProperties: false,
    },
    call: () => ({ method: "GET", path: "/api/v1/accounts" }),
  },
  {
    name: "outreach_add_cf_email_account",
    description:
      "Add a Cloudflare Email Sending account (FEATURE_CF_EMAIL). API token is vaulted and never returned. Transactional send; replies need Email Routing to this Worker. Does not send campaign mail.",
    inputSchema: {
      type: "object",
      properties: {
        email: { type: "string", description: "From address on a domain onboarded to Cloudflare Email Sending" },
        api_token: { type: "string", description: "Cloudflare API token with Email Sending Edit" },
        account_id: { type: "string", description: "Cloudflare account id" },
        daily_limit: { type: "number" },
      },
      required: ["email", "api_token", "account_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: "/api/v1/accounts/cf-email",
      body: {
        email: a.email,
        api_token: a.api_token,
        account_id: a.account_id,
        daily_limit: a.daily_limit,
      },
    }),
  },
  {
    name: "outreach_pause_account",
    description: "Pause a sending account so tick will not send from it.",
    inputSchema: {
      type: "object",
      properties: { account_id: { type: "string" } },
      required: ["account_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/accounts/${enc(a.account_id)}/pause`,
    }),
  },
  {
    name: "outreach_resume_account",
    description: "Resume a paused sending account.",
    inputSchema: {
      type: "object",
      properties: { account_id: { type: "string" } },
      required: ["account_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/accounts/${enc(a.account_id)}/resume`,
    }),
  },
  {
    name: "outreach_get_account_status",
    description: "Get status for one sending account (active, paused, reconnect_required).",
    inputSchema: {
      type: "object",
      properties: {
        account_id: { type: "string", description: "Account id" },
      },
      required: ["account_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "GET",
      path: `/api/v1/accounts/${enc(a.account_id)}/status`,
    }),
  },
  {
    name: "outreach_create_campaign",
    description:
      "Create a draft campaign. Does NOT send email. After create: add leads, preview, then ask a human to activate.",
    inputSchema: {
      type: "object",
      properties: {
        name: { type: "string" },
        sequence_yaml: { type: "string", description: "Sequence YAML content" },
        accounts: {
          type: "array",
          items: { type: "string" },
          description: "Sending account emails",
        },
        leads_csv: { type: "string" },
        open_tracking_enabled: { type: "boolean" },
        send_window_start: { type: "string" },
        send_window_end: { type: "string" },
        send_days: { type: "string" },
        timezone: { type: "string" },
      },
      required: ["name"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: "/api/v1/campaigns",
      body: {
        name: a.name,
        sequence_yaml: a.sequence_yaml,
        accounts: a.accounts || a.account_emails || [],
        leads_csv: a.leads_csv || "",
        open_tracking: a.open_tracking_enabled === true,
        draft_only: !a.leads_csv && !a.sequence_yaml ? true : false,
        send_window_start: a.send_window_start,
        send_window_end: a.send_window_end,
        send_days: a.send_days,
        timezone: a.timezone,
      },
    }),
  },
  {
    name: "outreach_update_campaign",
    description: "Update a draft or paused campaign. Does not activate or send.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        name: { type: "string" },
        sequence_yaml: { type: "string" },
        account_ids: { type: "array", items: { type: "string" } },
        open_tracking_enabled: { type: "boolean" },
      },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => {
      const { campaign_id, ...body } = a;
      return {
        method: "PATCH",
        path: `/api/v1/campaigns/${enc(campaign_id)}`,
        body,
      };
    },
  },
  {
    name: "outreach_preview_campaign",
    description:
      "Preview schedule and rendered sample messages for a campaign. Does not send.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
      },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "GET",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/preview`,
    }),
  },
  {
    name: "outreach_activate_campaign",
    description:
      "CONSEQUENTIAL: activates a campaign so the tick engine will begin sending scheduled emails. Requires explicit human approval. Creating or previewing a campaign does NOT send — only this tool (or the dashboard Activate action) starts sending. Do not call unless the user explicitly asked to activate.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        confirm: {
          type: "boolean",
          description: "Must be true to acknowledge consequential activation",
        },
      },
      required: ["campaign_id", "confirm"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/activate`,
      body: { confirm: a.confirm === true },
    }),
  },
  {
    name: "outreach_pause_campaign",
    description: "Pause an active campaign (cancels pending sends until resume).",
    inputSchema: {
      type: "object",
      properties: { campaign_id: { type: "string" } },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/pause`,
    }),
  },
  {
    name: "outreach_resume_campaign",
    description: "Resume a paused campaign.",
    inputSchema: {
      type: "object",
      properties: { campaign_id: { type: "string" } },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/resume`,
    }),
  },
  {
    name: "outreach_add_leads",
    description: "Add leads (CSV text or rows) to a campaign. Does not activate sending.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        csv: { type: "string", description: "CSV with email and placeholder columns" },
        leads: {
          type: "array",
          items: { type: "object", additionalProperties: { type: "string" } },
        },
      },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/leads`,
      body: { csv: a.csv, leads: a.leads },
    }),
  },
  {
    name: "outreach_remove_lead",
    description: "Remove a lead from a campaign and cancel its pending sends.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        lead_id: { type: "string" },
      },
      required: ["campaign_id", "lead_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "DELETE",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/leads/${enc(a.lead_id)}`,
    }),
  },
  {
    name: "outreach_validate_leads",
    description: "Validate lead CSV against campaign sequence placeholders before import.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        csv: { type: "string" },
        sequence_yaml: { type: "string" },
      },
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: "/api/v1/leads/validate",
      body: a,
    }),
  },
  {
    name: "outreach_get_campaign",
    description: "Get campaign details and status.",
    inputSchema: {
      type: "object",
      properties: { campaign_id: { type: "string" } },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "GET",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}`,
    }),
  },
  {
    name: "outreach_list_campaigns",
    description: "List campaigns in the workspace.",
    inputSchema: {
      type: "object",
      properties: {
        status: { type: "string", description: "Optional status filter" },
      },
      additionalProperties: false,
    },
    call: (a) => {
      const q = a.status ? `?status=${encodeURIComponent(String(a.status))}` : "";
      return { method: "GET", path: `/api/v1/campaigns${q}` };
    },
  },
  {
    name: "outreach_get_campaign_stats",
    description: "Get send/reply/open/bounce stats for a campaign.",
    inputSchema: {
      type: "object",
      properties: { campaign_id: { type: "string" } },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "GET",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/stats`,
    }),
  },
  {
    name: "outreach_list_replies",
    description: "List inbox threads with replies (outreach inbox).",
    inputSchema: {
      type: "object",
      properties: {
        limit: { type: "number" },
        cursor: { type: "string" },
      },
      additionalProperties: false,
    },
    call: (a) => {
      const params = new URLSearchParams();
      if (a.limit != null) params.set("limit", String(a.limit));
      if (a.cursor) params.set("cursor", String(a.cursor));
      const q = params.toString() ? `?${params}` : "";
      return { method: "GET", path: `/api/v1/inbox${q}` };
    },
  },
  {
    name: "outreach_get_thread",
    description: "Get one email thread with messages.",
    inputSchema: {
      type: "object",
      properties: { thread_id: { type: "string" } },
      required: ["thread_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "GET",
      path: `/api/v1/threads/${enc(a.thread_id)}`,
    }),
  },
  {
    name: "outreach_reply_to_thread",
    description:
      "Send a reply on an existing thread. Consequential: requires confirm=true and confirm_to matching the recipient. Unsubscribe/blacklist leads are refused.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        lead_id: { type: "string" },
        body: { type: "string" },
        confirm_to: { type: "string" },
        confirm: { type: "boolean" },
      },
      required: ["campaign_id", "lead_id", "body"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/threads/${enc(a.campaign_id)}/${enc(a.lead_id)}/reply`,
      body: {
        body: a.body,
        confirm_to: a.confirm_to,
        send: a.confirm === true,
        confirm: a.confirm === true,
      },
    }),
  },
  {
    name: "outreach_search_leads",
    description:
      "Search workspace leads, or connector-backed people search when provider is set (apollo). Connector search is preview-only.",
    inputSchema: {
      type: "object",
      properties: {
        q: { type: "string" },
        query: { type: "string" },
        limit: { type: "number" },
        provider: { type: "string" },
        credential_name: { type: "string" },
      },
      additionalProperties: false,
    },
    call: (a) => {
      const q = String(a.q ?? a.query ?? "");
      if (a.provider) {
        return {
          method: "POST",
          path: "/api/v1/integrations/search",
          body: {
            provider: a.provider,
            q,
            limit: a.limit,
            credential_name: a.credential_name,
          },
        };
      }
      const params = new URLSearchParams({ q });
      if (a.limit != null) params.set("limit", String(a.limit));
      return { method: "GET", path: `/api/v1/leads?${params}` };
    },
  },
  {
    name: "outreach_enrich_lead",
    description: "Enrich a lead by email via stored connector credentials (preview only).",
    inputSchema: {
      type: "object",
      properties: {
        email: { type: "string" },
        provider: { type: "string" },
        credential_name: { type: "string" },
      },
      required: ["email"],
      additionalProperties: false,
    },
    call: (a) => ({ method: "POST", path: "/api/v1/integrations/enrich", body: a }),
  },
  {
    name: "outreach_blacklist_lead",
    description:
      "Globally blacklist a lead (or domain) and cancel pending sends. Consequential suppression.",
    inputSchema: {
      type: "object",
      properties: {
        lead_id: { type: "string" },
        email: { type: "string" },
        domain: { type: "string" },
      },
      additionalProperties: false,
    },
    call: (a) => {
      if (a.lead_id) {
        return {
          method: "POST",
          path: `/api/v1/leads/${enc(a.lead_id)}/blacklist`,
        };
      }
      return {
        method: "POST",
        path: "/api/v1/leads/blacklist",
        body: { email: a.email, domain: a.domain },
      };
    },
  },
  {
    name: "outreach_list_capabilities",
    description: "List operator-enabled send/integration features for this instance (no secrets).",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
    call: () => ({ method: "GET", path: "/api/v1/settings/capabilities" }),
  },
  {
    name: "outreach_list_integrations",
    description: "List workspace integration credentials (masked). Never returns full secrets.",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
    call: () => ({ method: "GET", path: "/api/v1/integrations" }),
  },
  {
    name: "outreach_test_integration",
    description: "Test an integration credential by id without returning the secret.",
    inputSchema: {
      type: "object",
      properties: { id: { type: "number" } },
      required: ["id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/integrations/${enc(a.id)}/test`,
      body: {},
    }),
  },
  {
    name: "outreach_put_integration",
    description:
      "Create or rotate a workspace integration credential (Apollo, Clay, webhook, warmup, etc.). Never returns the secret.",
    inputSchema: {
      type: "object",
      properties: {
        provider: { type: "string" },
        name: { type: "string" },
        secret: { type: "string" },
        metadata: { type: "string" },
      },
      required: ["provider", "secret"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: "/api/v1/integrations",
      body: {
        provider: a.provider,
        name: a.name || "default",
        secret: a.secret,
        metadata: a.metadata,
      },
    }),
  },
  {
    name: "outreach_delete_integration",
    description: "Delete a workspace integration credential by id. Never returns secrets.",
    inputSchema: {
      type: "object",
      properties: { id: { type: "number" } },
      required: ["id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "DELETE",
      path: `/api/v1/integrations/${enc(a.id)}`,
    }),
  },
  {
    name: "outreach_apollo_search",
    description:
      "Search Apollo people using a stored apollo credential (preview only). Import via outreach_add_leads; never auto-activates.",
    inputSchema: {
      type: "object",
      properties: {
        credential_name: { type: "string" },
        q_keywords: { type: "string" },
        person_titles: { type: "array", items: { type: "string" } },
        per_page: { type: "number" },
      },
      additionalProperties: false,
    },
    call: (a) => ({ method: "POST", path: "/api/v1/integrations/apollo/search", body: a }),
  },
  {
    name: "outreach_sheets_import",
    description:
      "Import leads from a public Google Sheets (or CSV) URL. Preview without campaign_id; append with campaign_id. Does not activate.",
    inputSchema: {
      type: "object",
      properties: {
        url: { type: "string" },
        campaign_id: { type: "number" },
      },
      required: ["url"],
      additionalProperties: false,
    },
    call: (a) => ({ method: "POST", path: "/api/v1/integrations/sheets/import", body: a }),
  },
  {
    name: "outreach_import_leads",
    description: "Import CSV leads into a draft campaign. Does not activate.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        csv: { type: "string" },
        dry_run: { type: "boolean" },
        confirm: { type: "boolean" },
      },
      required: ["campaign_id", "csv"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/leads`,
      body: { csv: a.csv, dry_run: a.dry_run === true, confirm: a.confirm === true },
    }),
  },
  {
    name: "outreach_draft_sequence",
    description: "Draft sequence YAML from ICP/offer. Draft only — never activates.",
    inputSchema: {
      type: "object",
      properties: {
        icp: { type: "string" },
        offer: { type: "string" },
        tone: { type: "string" },
        step_count: { type: "number" },
        from_name: { type: "string" },
        campaign_id: { type: "number" },
      },
      additionalProperties: false,
    },
    call: (a) => ({ method: "POST", path: "/api/v1/agent/draft-sequence", body: a }),
  },
  {
    name: "outreach_preflight_campaign",
    description: "Non-mutating deliverability/readiness checks before activate.",
    inputSchema: {
      type: "object",
      properties: { campaign_id: { type: "string" } },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "GET",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/preflight`,
    }),
  },
  {
    name: "outreach_suggest_reply",
    description: "Suggest a reply body from classification. Sending still requires confirm on reply tool.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        lead_id: { type: "string" },
      },
      required: ["campaign_id", "lead_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "GET",
      path: `/api/v1/threads/${enc(a.campaign_id)}/${enc(a.lead_id)}/suggest-reply`,
    }),
  },
  {
    name: "outreach_setup",
    description: "First-run checklist: account/campaign/lead counts and next actions. Does not send.",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
    call: () => ({ method: "GET", path: "/api/v1/setup" }),
  },
  {
    name: "outreach_clone_campaign",
    description: "Clone a campaign as a new draft (never activates). Copies leads unless leads_csv is provided.",
    inputSchema: {
      type: "object",
      properties: {
        campaign_id: { type: "string" },
        name: { type: "string" },
        leads_csv: { type: "string" },
      },
      required: ["campaign_id"],
      additionalProperties: false,
    },
    call: (a) => ({
      method: "POST",
      path: `/api/v1/campaigns/${enc(a.campaign_id)}/clone`,
      body: { name: a.name, leads_csv: a.leads_csv },
    }),
  },
  {
    name: "outreach_list_suppressions",
    description: "List workspace suppression emails/domains.",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
    call: () => ({ method: "GET", path: "/api/v1/suppressions" }),
  },
  {
    name: "outreach_add_suppression",
    description: "Add an email or domain to the global suppression list. Honored on future imports. Does not send.",
    inputSchema: {
      type: "object",
      properties: {
        email: { type: "string" },
        domain: { type: "string" },
        kind: { type: "string" },
        value: { type: "string" },
      },
      additionalProperties: false,
    },
    call: (a) => ({ method: "POST", path: "/api/v1/suppressions", body: a }),
  },
  {
    name: "outreach_verify_leads",
    description: "Syntax + MX + disposable check. No third-party API key.",
    inputSchema: {
      type: "object",
      properties: {
        emails: { type: "array", items: { type: "string" } },
        csv: { type: "string" },
        email: { type: "string" },
      },
      additionalProperties: false,
    },
    call: (a) => ({ method: "POST", path: "/api/v1/leads/verify", body: a }),
  },
];

function enc(v: unknown): string {
  return encodeURIComponent(String(v ?? ""));
}

function stripSecrets(value: unknown): unknown {
  if (value == null) return value;
  if (Array.isArray(value)) return value.map(stripSecrets);
  if (typeof value !== "object") return value;
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    const key = k.toLowerCase();
    if (
      key.includes("token") ||
      key.includes("refresh") ||
      key.includes("secret") ||
      key.includes("password") ||
      key === "access_token" ||
      key === "authorization"
    ) {
      continue;
    }
    out[k] = stripSecrets(v);
  }
  return out;
}

function requireAuth(request: Request, env: McpEnv): Response | null {
  const expected = env.MCP_BEARER_TOKEN;
  if (!expected) return null;
  const auth = request.headers.get("Authorization") || "";
  const match = /^Bearer\s+(.+)$/i.exec(auth);
  if (!match || match[1] !== expected) {
    return new Response(JSON.stringify({ error: "unauthorized" }), {
      status: 401,
      headers: { "Content-Type": "application/json" },
    });
  }
  return null;
}

type ContainerGetter = (
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  binding: DurableObjectNamespace<any>,
  name?: string,
) => { fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response> };

async function proxyToContainer(
  env: McpEnv,
  getContainer: ContainerGetter,
  method: string,
  path: string,
  body?: unknown,
): Promise<unknown> {
  const container = getContainer(env.OUTREACH, "core");
  const headers: Record<string, string> = {
    Accept: "application/json",
  };
  if (env.INTERNAL_CONTAINER_TOKEN) {
    headers["X-Internal-Token"] = env.INTERNAL_CONTAINER_TOKEN;
  }
  let init: RequestInit = { method, headers };
  if (body !== undefined && method !== "GET" && method !== "DELETE") {
    headers["Content-Type"] = "application/json";
    init = { ...init, body: JSON.stringify(body), headers };
  }
  const res = await container.fetch(new Request(`http://container${path}`, init));
  const text = await res.text();
  let data: unknown = text;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { raw: text, status: res.status };
  }
  if (!res.ok) {
    return stripSecrets({
      error: true,
      status: res.status,
      data,
    });
  }
  return stripSecrets(data);
}

function toolsListResult() {
  return {
    tools: TOOLS.map((t) => ({
      name: t.name,
      description: t.description,
      inputSchema: t.inputSchema,
    })),
  };
}

async function toolsCall(
  env: McpEnv,
  getContainer: ContainerGetter,
  params: Record<string, unknown> | undefined,
): Promise<{ content: Array<{ type: string; text: string }>; isError?: boolean }> {
  const name = String(params?.name ?? "");
  const args = (params?.arguments as Record<string, unknown>) || {};
  const tool = TOOLS.find((t) => t.name === name);
  if (!tool) {
    return {
      content: [{ type: "text", text: JSON.stringify({ error: `unknown tool: ${name}` }) }],
      isError: true,
    };
  }
  if (name === "outreach_reply_to_thread" && args.confirm !== true) {
    return {
      content: [
        {
          type: "text",
          text: JSON.stringify({
            error: "reply_not_confirmed",
            message: "Set confirm=true and confirm_to after explicit human approval. Suggestion tools never send.",
          }),
        },
      ],
      isError: true,
    };
  }
  if (name === "outreach_activate_campaign" && args.confirm !== true) {
    return {
      content: [
        {
          type: "text",
          text: JSON.stringify({
            error: "activation_not_confirmed",
            message:
              "Set confirm=true only after explicit human approval. Creating a campaign does not send mail.",
          }),
        },
      ],
      isError: true,
    };
  }
  const req = tool.call(args);
  const data = await proxyToContainer(env, getContainer, req.method, req.path, req.body);
  return {
    content: [{ type: "text", text: JSON.stringify(data, null, 2) }],
  };
}

export async function handleMcp(
  request: Request,
  env: McpEnv,
  getContainer: ContainerGetter,
): Promise<Response> {
  const unauthorized = requireAuth(request, env);
  if (unauthorized) return unauthorized;

  if (request.method === "GET") {
    // Minimal discovery for Streamable HTTP clients
    return Response.json({
      name: "openoutreach",
      version: "0.1.0",
      protocol: "mcp",
      capabilities: { tools: {} },
    });
  }

  if (request.method !== "POST") {
    return new Response("Method Not Allowed", { status: 405 });
  }

  let rpc: JsonRpcRequest;
  try {
    rpc = (await request.json()) as JsonRpcRequest;
  } catch {
    return Response.json(
      { jsonrpc: "2.0", id: null, error: { code: -32700, message: "Parse error" } },
      { status: 400 },
    );
  }

  const id = rpc.id ?? null;
  const method = rpc.method || "";

  try {
    if (method === "initialize") {
      return Response.json({
        jsonrpc: "2.0",
        id,
        result: {
          protocolVersion: "2024-11-05",
          capabilities: { tools: {} },
          serverInfo: { name: "openoutreach", version: "0.1.0" },
        },
      });
    }
    if (method === "notifications/initialized" || method === "ping") {
      if (id === null || id === undefined) {
        return new Response(null, { status: 204 });
      }
      return Response.json({ jsonrpc: "2.0", id, result: {} });
    }
    if (method === "tools/list") {
      return Response.json({ jsonrpc: "2.0", id, result: toolsListResult() });
    }
    if (method === "tools/call") {
      const result = await toolsCall(env, getContainer, rpc.params);
      return Response.json({ jsonrpc: "2.0", id, result });
    }
    return Response.json({
      jsonrpc: "2.0",
      id,
      error: { code: -32601, message: `Method not found: ${method}` },
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return Response.json({
      jsonrpc: "2.0",
      id,
      error: { code: -32000, message },
    });
  }
}
