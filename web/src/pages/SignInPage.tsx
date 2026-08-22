import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { authClient, safeRedirect } from "../auth-client";

function GoogleLogo() {
  return (
    <svg aria-hidden="true" viewBox="0 0 18 18" width="16" height="16">
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.92c1.7-1.57 2.68-3.88 2.68-6.62Z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.33-1.58-5.04-3.72H.94v2.34A9 9 0 0 0 9 18Z"
      />
      <path
        fill="#FBBC05"
        d="M3.96 10.7A5.4 5.4 0 0 1 3.68 9c0-.59.1-1.16.28-1.7V4.96H.94A9 9 0 0 0 0 9c0 1.45.34 2.82.94 4.04l3.02-2.34Z"
      />
      <path
        fill="#EA4335"
        d="M9 3.58c1.32 0 2.5.45 3.44 1.35l2.58-2.58A8.64 8.64 0 0 0 9 0 9 9 0 0 0 .94 4.96L3.96 7.3C4.67 5.16 6.66 3.58 9 3.58Z"
      />
    </svg>
  );
}

export function AuthMark() {
  return (
    <div className="auth-mark" aria-hidden="true">
      OO
    </div>
  );
}

export function AuthCard({
  title,
  children,
  footer,
}: {
  title: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
}) {
  return (
    <div className="auth-shell">
      <div className="auth-card">
        <AuthMark />
        <h1>{title}</h1>
        {children}
        {footer ? <div className="auth-footer">{footer}</div> : null}
      </div>
    </div>
  );
}

export function AuthMethodChooser({
  googleLabel,
  emailLabel,
  isBusy,
  onGoogle,
  onEmail,
}: {
  googleLabel: string;
  emailLabel: string;
  isBusy?: boolean;
  onGoogle: () => void;
  onEmail: () => void;
}) {
  return (
    <div className="auth-methods">
      <button type="button" className="auth-google" onClick={onGoogle} disabled={isBusy}>
        <GoogleLogo />
        {isBusy ? "Opening Google..." : googleLabel}
      </button>
      <button type="button" className="secondary auth-email-btn" onClick={onEmail} disabled={isBusy}>
        {emailLabel}
      </button>
    </div>
  );
}

export default function SignInPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const redirectTo = safeRedirect(params.get("redirect"));
  const [showEmail, setShowEmail] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [googleBusy, setGoogleBusy] = useState(false);

  async function handleGoogle() {
    setError(null);
    setGoogleBusy(true);
    try {
      const result = await authClient.signIn.social({
        provider: "google",
        callbackURL: redirectTo,
      });
      if (result.error) {
        setError(result.error.message || "Google sign in is not available right now.");
        setGoogleBusy(false);
      }
    } catch {
      setError("Google sign in is not available right now.");
      setGoogleBusy(false);
    }
  }

  async function handleEmail(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const result = await authClient.signIn.email({
        email: email.trim(),
        password,
        callbackURL: redirectTo,
      });
      if (!result.error) {
        navigate(redirectTo, { replace: true });
        return;
      }
      setError(result.error.message || "We couldn't sign you in.");
    } catch {
      setError("Unable to sign in right now. Please try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard
      title="Sign in"
      footer={
        showEmail ? (
          <div className="auth-footer-row">
            <button type="button" className="linkish" onClick={() => setShowEmail(false)}>
              Back
            </button>
            <Link to={`/sign-up?redirect=${encodeURIComponent(redirectTo)}`}>Create account</Link>
          </div>
        ) : (
          <Link to={`/sign-up?redirect=${encodeURIComponent(redirectTo)}`}>Create account</Link>
        )
      }
    >
      {!showEmail ? (
        <>
          <AuthMethodChooser
            googleLabel="Continue with Google"
            emailLabel="Continue with email"
            isBusy={googleBusy}
            onGoogle={() => void handleGoogle()}
            onEmail={() => {
              setShowEmail(true);
              setError(null);
            }}
          />
          {error ? <p className="auth-error">{error}</p> : null}
        </>
      ) : (
        <form className="auth-form" onSubmit={(e) => void handleEmail(e)}>
          <input
            type="email"
            placeholder="Email address..."
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
            required
          />
          <input
            type="password"
            placeholder="Password..."
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
          {error ? <p className="auth-error">{error}</p> : null}
          <button type="submit" disabled={busy}>
            {busy ? "Signing in..." : "Sign in"}
          </button>
        </form>
      )}
    </AuthCard>
  );
}
