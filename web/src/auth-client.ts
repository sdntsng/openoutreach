import { createAuthClient } from "better-auth/react";
import { useEffect, useState } from "react";

export type AuthMode = "cloudflare_access" | "hosted" | "local_noauth";

export type AuthUser = { email: string; name?: string };

export type WhoAmI = {
  mode: AuthMode;
  user: AuthUser | null;
  accessConfigured?: boolean;
};

export const authClient = createAuthClient({
  baseURL: typeof window !== "undefined" ? window.location.origin : "",
});

export const { useSession } = authClient;

export function safeRedirect(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//")) return "/";
  if (value.startsWith("/sign-in") || value.startsWith("/sign-up")) return "/";
  return value;
}

export function useAuth(): WhoAmI & { pending: boolean } {
  const [state, setState] = useState<WhoAmI & { pending: boolean }>({
    pending: true,
    mode: "cloudflare_access",
    user: null,
  });

  useEffect(() => {
    let cancelled = false;
    fetch("/api/auth/whoami", { credentials: "include", headers: { Accept: "application/json" } })
      .then(async (res) => {
        const data = (await res.json()) as WhoAmI;
        if (!cancelled) setState({ pending: false, ...data });
      })
      .catch(() => {
        if (!cancelled) {
          setState({ pending: false, mode: "local_noauth", user: { email: "local", name: "Local" } });
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return state;
}

export function signOut(mode: AuthMode) {
  if (mode === "cloudflare_access") {
    window.location.assign("/cdn-cgi/access/logout");
    return;
  }
  if (mode === "hosted") {
    void authClient.signOut({
      fetchOptions: {
        onSuccess: () => {
          window.location.assign("/sign-in");
        },
      },
    });
  }
}
