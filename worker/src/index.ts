import { Container, getContainer } from "@cloudflare/containers";
import { verifyAccessJwt } from "./access";
import { handleMcp } from "./mcp";

/** Transparent 1x1 GIF */
const PIXEL_GIF_B64 =
  "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

export interface Env {
  OUTREACH: DurableObjectNamespace<OutreachContainer>;
  ASSETS: Fetcher;
  INTERNAL_CONTAINER_TOKEN?: string;
  MCP_BEARER_TOKEN?: string;
  PUBLIC_BASE_URL?: string;
  /** When set, require verified Access JWT or MCP bearer (except public paths). */
  CF_ACCESS_AUD?: string;
  DATABASE_URL?: string;
  GOOGLE_CLIENT_ID?: string;
  GOOGLE_CLIENT_SECRET?: string;
  CREDENTIAL_ENCRYPTION_KEY?: string;
  GOOGLE_REDIRECT_URL?: string;
  OPENOUTREACH_WORKSPACE_ID?: string;
}

export class OutreachContainer extends Container<Env> {
  defaultPort = 8080;
  sleepAfter = "10m";
  enableInternet = true;

  envVars = {
    LISTEN_ADDR: ":8080",
  };

  override async start(
    options?: {
      envVars?: Record<string, string>;
      enableInternet?: boolean;
      entrypoint?: string[];
    },
    waitOptions?: { signal?: AbortSignal },
  ): Promise<void> {
    const e = this.env;
    const forwarded: Record<string, string> = {
      LISTEN_ADDR: ":8080",
      ...(options?.envVars || {}),
    };
    const map: Array<[keyof Env, string]> = [
      ["DATABASE_URL", "COLD_CLI_DATABASE_URL"],
      ["DATABASE_URL", "DATABASE_URL"],
      ["GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_ID"],
      ["GOOGLE_CLIENT_SECRET", "GOOGLE_CLIENT_SECRET"],
      ["CREDENTIAL_ENCRYPTION_KEY", "CREDENTIAL_ENCRYPTION_KEY"],
      ["INTERNAL_CONTAINER_TOKEN", "INTERNAL_CONTAINER_TOKEN"],
      ["PUBLIC_BASE_URL", "PUBLIC_BASE_URL"],
      ["GOOGLE_REDIRECT_URL", "GOOGLE_REDIRECT_URL"],
      ["OPENOUTREACH_WORKSPACE_ID", "OPENOUTREACH_WORKSPACE_ID"],
    ];
    for (const [from, to] of map) {
      const v = e[from];
      if (typeof v === "string" && v) forwarded[to] = v;
    }
    return super.start(
      {
        ...options,
        envVars: forwarded,
        enableInternet: options?.enableInternet ?? true,
      },
      waitOptions,
    );
  }
}

function pixelResponse(): Response {
  const bytes = Uint8Array.from(atob(PIXEL_GIF_B64), (c) => c.charCodeAt(0));
  return new Response(bytes, {
    status: 200,
    headers: {
      "Content-Type": "image/gif",
      "Cache-Control": "no-store, no-cache, must-revalidate, private",
      "Content-Length": String(bytes.byteLength),
    },
  });
}

function containerStub(env: Env) {
  return getContainer(env.OUTREACH, "default");
}

function isPublicPath(pathname: string): boolean {
  return (
    pathname.startsWith("/t/") ||
    pathname.startsWith("/api/v1/accounts/google/oauth/callback")
  );
}

function unauthorized(): Response {
  return new Response(JSON.stringify({ error: { code: "unauthorized", message: "Access required" } }), {
    status: 401,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * Access gate: when CF_ACCESS_AUD is configured, require a verified Access JWT,
 * MCP bearer for /mcp, or internal token for /internal. Tracking + OAuth callback stay public.
 */
async function accessGate(request: Request, env: Env, pathname: string): Promise<Response | null> {
  if (!env.CF_ACCESS_AUD || isPublicPath(pathname)) return null;

  if (pathname.startsWith("/mcp") && env.MCP_BEARER_TOKEN) {
    const auth = request.headers.get("Authorization") || "";
    if (auth === `Bearer ${env.MCP_BEARER_TOKEN}`) return null;
  }
  if (pathname.startsWith("/internal/") && env.INTERNAL_CONTAINER_TOKEN) {
    if (request.headers.get("X-Internal-Token") === env.INTERNAL_CONTAINER_TOKEN) return null;
  }

  const jwt = request.headers.get("Cf-Access-Jwt-Assertion");
  if (jwt && (await verifyAccessJwt(jwt, env.CF_ACCESS_AUD))) return null;

  return unauthorized();
}

async function proxyToContainer(
  request: Request,
  env: Env,
  pathOverride?: string,
): Promise<Response> {
  const url = new URL(request.url);
  const path = pathOverride ?? url.pathname + url.search;
  const headers = new Headers(request.headers);
  headers.delete("host");
  if (env.INTERNAL_CONTAINER_TOKEN) {
    headers.set("X-Internal-Token", env.INTERNAL_CONTAINER_TOKEN);
  }
  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  const body = hasBody ? await request.arrayBuffer() : undefined;
  return containerStub(env).fetch(
    new Request(`http://container${path}`, {
      method: request.method,
      headers,
      body,
    }),
  );
}

async function recordOpen(env: Env, token: string, request: Request): Promise<void> {
  try {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "application/json",
    };
    if (env.INTERNAL_CONTAINER_TOKEN) {
      headers["X-Internal-Token"] = env.INTERNAL_CONTAINER_TOKEN;
    }
    await containerStub(env).fetch(
      new Request("http://container/api/v1/internal/track/open", {
        method: "POST",
        headers,
        body: JSON.stringify({
          token,
          user_agent: request.headers.get("user-agent") || "",
          country: request.headers.get("cf-ipcountry") || "",
        }),
      }),
    );
  } catch {
    // pixel must still succeed
  }
}

async function recordClick(
  env: Env,
  _messageToken: string,
  linkToken: string,
  request: Request,
): Promise<{ redirect_url?: string } | null> {
  try {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "application/json",
    };
    if (env.INTERNAL_CONTAINER_TOKEN) {
      headers["X-Internal-Token"] = env.INTERNAL_CONTAINER_TOKEN;
    }
    const res = await containerStub(env).fetch(
      new Request("http://container/api/v1/internal/track/click", {
        method: "POST",
        headers,
        body: JSON.stringify({
          token: linkToken,
          user_agent: request.headers.get("user-agent") || "",
          country: request.headers.get("cf-ipcountry") || "",
        }),
      }),
    );
    if (!res.ok) return null;
    const payload = (await res.json()) as {
      data?: { destination_url?: string };
      destination_url?: string;
    };
    return {
      redirect_url:
        payload.data?.destination_url || payload.destination_url || undefined,
    };
  } catch {
    return null;
  }
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const { pathname } = url;

    const denied = await accessGate(request, env, pathname);
    if (denied) return denied;

    const openMatch = pathname.match(/^\/t\/o\/([^/]+?)(?:\.gif)?$/);
    if (openMatch && request.method === "GET") {
      const token = decodeURIComponent(openMatch[1].replace(/\.gif$/i, ""));
      const recordPromise = recordOpen(env, token, request);
      const gif = pixelResponse();
      await recordPromise;
      return gif;
    }

    const clickMatch = pathname.match(/^\/t\/c\/([^/]+)\/([^/]+)$/);
    if (clickMatch && request.method === "GET") {
      const messageToken = decodeURIComponent(clickMatch[1]);
      const linkToken = decodeURIComponent(clickMatch[2]);
      const result = await recordClick(env, messageToken, linkToken, request);
      const dest = result?.redirect_url || url.searchParams.get("u") || "/";
      return Response.redirect(dest, 302);
    }

    if (pathname === "/mcp" || pathname.startsWith("/mcp/")) {
      return handleMcp(request, env, getContainer as never);
    }

    if (pathname.startsWith("/api/") || pathname.startsWith("/internal/")) {
      return proxyToContainer(request, env);
    }

    return env.ASSETS.fetch(request);
  },

  async scheduled(
    _controller: ScheduledController,
    env: Env,
    _ctx: ExecutionContext,
  ): Promise<void> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "application/json",
    };
    if (env.INTERNAL_CONTAINER_TOKEN) {
      headers["X-Internal-Token"] = env.INTERNAL_CONTAINER_TOKEN;
    }
    await containerStub(env).fetch(
      new Request("http://container/internal/tick", {
        method: "POST",
        headers,
        body: "{}",
      }),
    );
  },
};
