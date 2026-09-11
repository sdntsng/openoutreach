import { APIError } from "better-auth/api";
import { betterAuth } from "better-auth";
import { acceptInvite, emailMayJoin } from "./team";

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
  OPENOUTREACH_WORKSPACE_ID?: string;
};

export function googleAuthConfigured(env: AuthEnv): boolean {
  return Boolean(env.GOOGLE_CLIENT_ID?.trim() && env.GOOGLE_CLIENT_SECRET?.trim());
}

export function createAuth(env: AuthEnv, request: Request) {
  const secret = (env.BETTER_AUTH_SECRET || env.CREDENTIAL_ENCRYPTION_KEY || "").trim();
  if (!env.DB || secret.length < 32) return null;

  const baseURL = (env.PUBLIC_BASE_URL || new URL(request.url).origin).replace(/\/$/, "");
  const googleOn = googleAuthConfigured(env);

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
      minPasswordLength: 8,
    },
    socialProviders: googleOn
      ? {
          google: {
            clientId: env.GOOGLE_CLIENT_ID!.trim(),
            clientSecret: env.GOOGLE_CLIENT_SECRET!.trim(),
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
            if (!(await emailMayJoin(env, user.email))) {
              throw new APIError("FORBIDDEN", {
                message: "This project is invite-only. Ask an owner for a link.",
              });
            }
            return { data: user };
          },
          after: async (user) => {
            await acceptInvite(env, user.email);
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
