import { Container, getContainer } from "@cloudflare/containers";
import { accessEmail, createAuth, getSession } from "./auth";
import { accessAudience, getAuthMode, type AuthMode } from "./auth-mode";
import { containerEnv } from "./container-env";
import { handleD1 } from "./d1";
import { handleMcp } from "./mcp";

/** Transparent 1x1 GIF */
const PIXEL_GIF_B64 =
  "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

export interface Env {
  OUTREACH: DurableObjectNamespace<OutreachContainer>;
  ASSETS: Fetcher;
  DB?: D1Database;
  INTERNAL_CONTAINER_TOKEN?: string;
  TRACKING_HMAC_SECRET?: string;
  MCP_BEARER_TOKEN?: string;
  PUBLIC_BASE_URL?: string;
  /** When set, require Access JWT assertion or MCP bearer (except public paths). */
  CF_ACCESS_AUD?: string;
  DATABASE_URL?: string;
  GOOGLE_CLIENT_ID?: string;
  GOOGLE_CLIENT_SECRET?: string;
  CREDENTIAL_ENCRYPTION_KEY?: string;
  GOOGLE_REDIRECT_URL?: string;
  OPENOUTREACH_WORKSPACE_ID?: string;
  BETTER_AUTH_SECRET?: string;
  AUTH_ALLOWED_EMAILS?: string;
  AUTH_MODE?: string;
  POLICY_AUD?: string;
  TEAM_DOMAIN?: string;
}

export class OutreachContainer extends Container<Env> {
  defaultPort = 8080;
  sleepAfter = "10m";
  enableInternet = true;

  override onError(error: unknown): void {
    console.error("outreach container error", error);
  }

  override onStart(): void {
    console.log("outreach container started");
  }

  envVars: Record<string, string> = { ...containerEnv };

  private containerEnvForStart(options?: { envVars?: Record<string, string> }): Record<string, string> {
    const e = this.env;
    const forwarded: Record<string, string> = {
      ...containerEnv,
      LISTEN_ADDR: ":8080",
      ...(options?.envVars || {}),
    };
    const map: Array<[keyof Env, string]> = [
      ["GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_ID"],
      ["GOOGLE_CLIENT_SECRET", "GOOGLE_CLIENT_SECRET"],
      ["CREDENTIAL_ENCRYPTION_KEY", "CREDENTIAL_ENCRYPTION_KEY"],
      ["INTERNAL_CONTAINER_TOKEN", "INTERNAL_CONTAINER_TOKEN"],
      ["PUBLIC_BASE_URL", "PUBLIC_BASE_URL"],
      ["TRACKING_HMAC_SECRET", "TRACKING_HMAC_SECRET"],
      ["GOOGLE_REDIRECT_URL", "GOOGLE_REDIRECT_URL"],
      ["OPENOUTREACH_WORKSPACE_ID", "OPENOUTREACH_WORKSPACE_ID"],
    ];
    for (const [from, to] of map) {
      const v = e[from];
      if (typeof v === "string" && v) forwarded[to] = v;
    }
    if (typeof e.DATABASE_URL === "string" && e.DATABASE_URL) {
      forwarded.COLD_CLI_DATABASE_URL = e.DATABASE_URL;
      forwarded.DATABASE_URL = e.DATABASE_URL;
    } else if (typeof e.PUBLIC_BASE_URL === "string" && e.PUBLIC_BASE_URL) {
      forwarded.OPENOUTREACH_D1_PROXY = e.PUBLIC_BASE_URL.replace(/\/$/, "");
    }
    return forwarded;
  }

  /** Stop warm containers when deploy bumps CONTAINER_BOOT_REVISION. */
  private async ensureBootRevision(): Promise<void> {
    const revision = containerEnv.CONTAINER_BOOT_REVISION ?? "";
    const stored = await this.ctx.storage.get<string>("boot_revision");
    if (stored === revision) return;
    // Only stop a previously-warm container when revision changes. Do not stop on
    // first boot (stored unset) or we interrupt outreachd schema migration.
    if (stored != null && stored !== revision) {
      const state = await this.getState();
      if (state.status === "running" || state.status === "healthy") {
        await this.stop();
      }
    }
    await this.ctx.storage.put("boot_revision", revision);
  }

  override async fetch(request: Request): Promise<Response> {
    await this.ensureBootRevision();
    return super.fetch(request);
  }

  override async start(
    options?: {
      envVars?: Record<string, string>;
      enableInternet?: boolean;
      entrypoint?: string[];
    },
    waitOptions?: { signal?: AbortSignal },
  ): Promise<void> {
    const forwarded = this.containerEnvForStart(options);
    this.envVars = forwarded;
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
  return getContainer(env.OUTREACH, "core");
}

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function isPublicPath(pathname: string, mode: AuthMode): boolean {
  if (pathname.startsWith("/t/")) return true;
  if (pathname === "/oauth/google/callback" || pathname.startsWith("/oauth/google/")) return true;
  if (pathname.startsWith("/api/v1/accounts/google/oauth/callback")) return true;
  if (pathname.startsWith("/api/v1/accounts/microsoft/oauth/callback")) return true;
  if (pathname.match(/^\/api\/v1\/integrations\/[^/]+\/ingest$/)) return true;
  if (pathname === "/api/auth/whoami") return true;
  if (mode === "hosted") {
    return pathname.startsWith("/api/auth") || pathname === "/sign-in" || pathname === "/sign-up";
  }
  return false;
}

function isAuthPage(pathname: string): boolean {
  return pathname === "/sign-in" || pathname === "/sign-up";
}

function isStaticAsset(pathname: string): boolean {
  return (
    pathname.startsWith("/assets/") ||
    pathname === "/favicon.ico" ||
    pathname === "/robots.txt"
  );
}

function hasMcpBearer(request: Request, env: Env): boolean {
  if (!env.MCP_BEARER_TOKEN) return false;
  return (request.headers.get("Authorization") || "") === `Bearer ${env.MCP_BEARER_TOKEN}`;
}

function hasInternalToken(request: Request, env: Env): boolean {
  if (!env.INTERNAL_CONTAINER_TOKEN) return false;
  return request.headers.get("X-Internal-Token") === env.INTERNAL_CONTAINER_TOKEN;
}

/**
 * Cloudflare Access gate (default AUTH_MODE). Tracking, Gmail callback, and
 * internal-token routes stay public. MCP may use a bearer token.
 */
function accessGate(request: Request, env: Env, pathname: string, mode: AuthMode): Response | null {
  if (mode !== "cloudflare_access") return null;
  if (isPublicPath(pathname, mode) || isStaticAsset(pathname)) return null;
  if (pathname.startsWith("/internal/") && hasInternalToken(request, env)) return null;
  if (pathname.startsWith("/mcp") && hasMcpBearer(request, env)) return null;

  const aud = accessAudience(env);
  if (!aud) {
    if (pathname.startsWith("/api/") || pathname.startsWith("/mcp")) {
      return json(
        {
          error: {
            code: "access_unconfigured",
            message: "AUTH_MODE=cloudflare_access but CF_ACCESS_AUD is not set. Create an Access application or set AUTH_MODE=hosted / local_noauth.",
          },
        },
        503,
      );
    }
    return new Response(
      `<!doctype html><meta charset="utf-8"><title>Access not configured</title>
<style>body{font:14px/1.45 Inter,system-ui,sans-serif;max-width:36rem;margin:12vh auto;padding:0 1.5rem;color:#1c2421}
code{font:12px ui-monospace,Menlo,monospace;background:#f0f5f3;padding:.1rem .35rem;border-radius:6px}</style>
<h1>Cloudflare Access is not configured</h1>
<p>This instance defaults to <code>AUTH_MODE=cloudflare_access</code>. Create a self-hosted Access application for this hostname, set <code>CF_ACCESS_AUD</code> to its AUD tag, and allow your email. Bypass <code>/t/*</code>, <code>/internal/*</code>, and the Gmail OAuth callback.</p>
<p>Or set <code>AUTH_MODE=hosted</code> for Better Auth (Google / email), or <code>AUTH_MODE=local_noauth</code> on a private network.</p>`,
      { status: 503, headers: { "Content-Type": "text/html; charset=utf-8" } },
    );
  }

  if (request.headers.get("Cf-Access-Jwt-Assertion")) return null;
  return json({ error: { code: "unauthorized", message: "Access required" } }, 401);
}

async function handleWhoAmI(request: Request, env: Env, mode: AuthMode): Promise<Response> {
  if (mode === "local_noauth") {
    return json({ mode, user: { email: "local", name: "Local" } });
  }
  if (mode === "cloudflare_access") {
    const email = accessEmail(request);
    const aud = accessAudience(env);
    return json({
      mode,
      accessConfigured: Boolean(aud),
      user: email ? { email, name: email } : null,
    });
  }
  const session = await getSession(env, request);
  const user = session?.user;
  return json({
    mode,
    user: user?.email ? { email: user.email, name: user.name || user.email } : null,
  });
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
    const mode = getAuthMode(env.AUTH_MODE);

    const denied = accessGate(request, env, pathname, mode);
    if (denied) return denied;

    if (pathname === "/api/auth/whoami") {
      return handleWhoAmI(request, env, mode);
    }

    if (pathname.startsWith("/api/auth")) {
      if (mode !== "hosted") {
        return json({ error: { code: "not_found", message: "Better Auth is disabled (AUTH_MODE is not hosted)" } }, 404);
      }
      const auth = createAuth(env, request);
      if (!auth) {
        return json({ error: { code: "auth_unconfigured", message: "Sign-in is not configured" } }, 501);
      }
      return auth.handler(request);
    }

    if (pathname === "/internal/d1") {
      return handleD1(request, env);
    }

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
      if (hasMcpBearer(request, env)) {
        return handleMcp(request, env, getContainer as never);
      }
      if (mode === "hosted") {
        const session = await getSession(env, request);
        if (!session) {
          return json({ error: { code: "unauthorized", message: "Sign in required" } }, 401);
        }
      }
      return handleMcp(request, env, getContainer as never);
    }

    if (mode === "hosted") {
      const skipSession =
        isPublicPath(pathname, mode) ||
        isAuthPage(pathname) ||
        isStaticAsset(pathname) ||
        pathname.startsWith("/internal/");
      if (!skipSession) {
        const auth = createAuth(env, request);
        if (auth) {
          const session = await auth.api.getSession({ headers: request.headers });
          if (!session) {
            if (pathname.startsWith("/api/")) {
              return json({ error: { code: "unauthorized", message: "Sign in required" } }, 401);
            }
            const next = pathname + url.search;
            return Response.redirect(
              new URL(`/sign-in?redirect=${encodeURIComponent(next)}`, url.origin).href,
              302,
            );
          }
        }
      }
    }

    if (
      pathname.startsWith("/api/") ||
      pathname.startsWith("/internal/") ||
      pathname === "/oauth/google/callback" ||
      pathname.startsWith("/oauth/google/")
    ) {
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
