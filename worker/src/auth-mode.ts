export const AUTH_MODES = ["cloudflare_access", "hosted", "local_noauth"] as const;
export type AuthMode = (typeof AUTH_MODES)[number];

export function getAuthMode(value: string | null | undefined): AuthMode {
  const v = (value || "").trim().toLowerCase();
  if (v === "hosted" || v === "better_auth") return "hosted";
  if (v === "local_noauth") return "local_noauth";
  return "cloudflare_access";
}

export function accessAudience(env: { CF_ACCESS_AUD?: string; POLICY_AUD?: string }): string {
  return (env.CF_ACCESS_AUD || env.POLICY_AUD || "").trim();
}
