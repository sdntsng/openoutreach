import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { authClient, safeRedirect } from "../auth-client";
import { AuthCard, AuthMethodChooser } from "./SignInPage";

export default function SignUpPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const redirectTo = safeRedirect(params.get("redirect"));
  const [showEmail, setShowEmail] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
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
    if (password.length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }
    if (password !== confirm) {
      setError("Passwords do not match.");
      return;
    }
    setBusy(true);
    try {
      const result = await authClient.signUp.email({
        name: name.trim() || email.trim(),
        email: email.trim(),
        password,
        callbackURL: redirectTo,
      });
      if (!result.error) {
        navigate(redirectTo, { replace: true });
        return;
      }
      setError(result.error.message || "We couldn't create your account.");
    } catch {
      setError("Unable to sign up right now. Please try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard
      title="Create account"
      footer={
        showEmail ? (
          <div className="auth-footer-row">
            <button type="button" className="linkish" onClick={() => setShowEmail(false)}>
              Back
            </button>
            <Link to={`/sign-in?redirect=${encodeURIComponent(redirectTo)}`}>Sign in</Link>
          </div>
        ) : (
          <Link to={`/sign-in?redirect=${encodeURIComponent(redirectTo)}`}>Sign in</Link>
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
            type="text"
            placeholder="Name..."
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoComplete="name"
          />
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
            autoComplete="new-password"
            required
          />
          <input
            type="password"
            placeholder="Confirm password..."
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            required
          />
          {error ? <p className="auth-error">{error}</p> : null}
          <button type="submit" disabled={busy}>
            {busy ? "Creating account..." : "Create account"}
          </button>
        </form>
      )}
    </AuthCard>
  );
}
