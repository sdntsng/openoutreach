import { APIError } from "better-auth/api";
import { betterAuth } from "better-auth";

export type AuthEnv = {
  DB?: D1Database;
  PUBLIC_BASE_URL?: string;
  BETTER_AUTH_SECRET?: string;
  CREDENTIAL_ENCRYPTION_KEY?: string;
  GOOGLE_CLIENT_ID?: string;
  GOOGLE_CLIENT_SECRET?: string;
  AUTH_ALLOWED_EMAILS?: string;
  AUTH_MODE?: string;
  CF_ACCESS_AUD?: string;
  POLICY_AUD?: string;
};

function allowedEmails(env: AuthEnv): string[] {
  return (env.AUTH_ALLOWED_EMAILS || "")
    .split(",")
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
}

function isAllowedEmail(env: AuthEnv, email: string): boolean {
  const allow = allowedEmails(env);
  if (allow.length === 0) return true;
  return allow.includes(email.trim().toLowerCase());
}

export function createAuth(env: AuthEnv, request: Request) {
  const secret = (env.BETTER_AUTH_SECRET || env.CREDENTIAL_ENCRYPTION_KEY || "").trim();
  if (!env.DB || secret.length < 32) return null;

  const baseURL = (env.PUBLIC_BASE_URL || new URL(request.url).origin).replace(/\/$/, "");
  const googleId = env.GOOGLE_CLIENT_ID?.trim();
  const googleSecret = env.GOOGLE_CLIENT_SECRET?.trim();

  return betterAuth({
    baseURL,
    secret,
    database: env.DB,
    trustedOrigins: [baseURL],
    advanced: {
      ipAddress: {
        ipAddressHeaders: ["cf-connecting-ip"],
      },
    },
    emailAndPassword: {
      enabled: true,
      requireEmailVerification: false,
    },
    socialProviders:
      googleId && googleSecret
        ? {
            google: {
              clientId: googleId,
              clientSecret: googleSecret,
              mapProfileToUser: (profile: { name?: string; email?: string }) => ({
                name: profile.name || profile.email || "User",
              }),
            },
          }
        : undefined,
    databaseHooks: {
      user: {
        create: {
          before: async (user) => {
            if (!isAllowedEmail(env, user.email)) {
              throw new APIError("FORBIDDEN", {
                message: "This OpenOutreach instance is invite-only.",
              });
            }
            return { data: user };
          },
        },
      },
    },
  });
}

export async function getSession(env: AuthEnv, request: Request) {
  const auth = createAuth(env, request);
  if (!auth) return null;
  return auth.api.getSession({ headers: request.headers });
}

export function accessEmail(request: Request): string | null {
  return (
    request.headers.get("Cf-Access-Authenticated-User-Email") ||
    request.headers.get("cf-access-authenticated-user-email")
  );
}

