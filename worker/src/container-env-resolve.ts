import { containerEnv } from "./container-env";

/** Worker bindings that outreachd reads as process env. */
const FORWARD: Array<[string, string]> = [
  ["GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_ID"],
  ["GOOGLE_CLIENT_SECRET", "GOOGLE_CLIENT_SECRET"],
  ["CREDENTIAL_ENCRYPTION_KEY", "CREDENTIAL_ENCRYPTION_KEY"],
  ["INTERNAL_CONTAINER_TOKEN", "INTERNAL_CONTAINER_TOKEN"],
  ["PUBLIC_BASE_URL", "PUBLIC_BASE_URL"],
  ["TRACKING_HMAC_SECRET", "TRACKING_HMAC_SECRET"],
  ["GOOGLE_REDIRECT_URL", "GOOGLE_REDIRECT_URL"],
  ["OPENOUTREACH_WORKSPACE_ID", "OPENOUTREACH_WORKSPACE_ID"],
  ["AUTH_MODE", "AUTH_MODE"],
  ["MCP_BEARER_TOKEN", "MCP_BEARER_TOKEN"],
  ["MICROSOFT_CLIENT_ID", "MICROSOFT_CLIENT_ID"],
  ["MICROSOFT_CLIENT_SECRET", "MICROSOFT_CLIENT_SECRET"],
  ["MICROSOFT_TENANT_ID", "MICROSOFT_TENANT_ID"],
  ["FEATURE_GMAIL", "FEATURE_GMAIL"],
  ["FEATURE_MICROSOFT", "FEATURE_MICROSOFT"],
  ["FEATURE_SMTP_IMAP", "FEATURE_SMTP_IMAP"],
  ["FEATURE_APOLLO", "FEATURE_APOLLO"],
  ["FEATURE_CLAY", "FEATURE_CLAY"],
  ["FEATURE_WEBHOOK", "FEATURE_WEBHOOK"],
  ["FEATURE_SHEETS", "FEATURE_SHEETS"],
  ["FEATURE_RESEND", "FEATURE_RESEND"],
  ["FEATURE_SES", "FEATURE_SES"],
  ["FEATURE_CF_EMAIL", "FEATURE_CF_EMAIL"],
  ["FEATURE_WARMUP", "FEATURE_WARMUP"],
  ["FEATURE_HUNTER", "FEATURE_HUNTER"],
];

/**
 * Env for outreachd. Workers Builds cannot bake secrets into container-env.ts;
 * the Container SDK starts via startAndWaitForPorts (uses this.envVars), not start().
 */
export function resolveContainerEnv(
  env: Record<string, unknown>,
  extras?: Record<string, string>,
): Record<string, string> {
  const forwarded: Record<string, string> = {
    ...containerEnv,
    LISTEN_ADDR: ":8080",
    ...(extras || {}),
  };
  for (const [from, to] of FORWARD) {
    const v = env[from];
    if (typeof v === "string" && v) forwarded[to] = v;
  }
  if (typeof env.DATABASE_URL === "string" && env.DATABASE_URL) {
    forwarded.COLD_CLI_DATABASE_URL = env.DATABASE_URL;
    forwarded.DATABASE_URL = env.DATABASE_URL;
  } else if (typeof env.PUBLIC_BASE_URL === "string" && env.PUBLIC_BASE_URL) {
    forwarded.OPENOUTREACH_D1_PROXY = env.PUBLIC_BASE_URL.replace(/\/$/, "");
  }
  return forwarded;
}
