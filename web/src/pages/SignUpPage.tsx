import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api";
import { authClient, safeRedirect, useAuth } from "../auth-client";
import { AuthCard } from "./SignInPage";

export default function SignUpPage() {
  const auth = useAuth();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const redirectTo = safeRedirect(params.get("redirect"));
  const inviteToken = (params.get("invite") || "").trim();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [googleBusy, setGoogleBusy] = useState(false);
  const [inviteLocked, setInviteLocked] = useState(false);
  const googleOn = Boolean(auth.methods?.google) && !inviteToken;

  useEffect(() => {
    if (!inviteToken) return;
    api
      .lookupInvite(inviteToken)
      .then((d) => {
        setEmail(d.email);
        setInviteLocked(true);
      })
      .catch(() => setError("This invite is invalid or already used."));
  }, [inviteToken]);

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
      title={inviteLocked ? "Join this project" : "Create account"}
      footer={<Link to={`/sign-in?redirect=${encodeURIComponent(redirectTo)}`}>Sign in</Link>}
    >
      <p className="muted" style={{ marginTop: 0 }}>
        {inviteLocked
          ? "Set a password for this email. You will share this workspace — campaigns, mailbox, and leads."
          : "Sign up with email and password. This instance may be invite-only."}
      </p>
      {googleOn ? (
        <div className="auth-methods">
          <button type="button" className="auth-google" onClick={() => void handleGoogle()} disabled={googleBusy}>
            {googleBusy ? "Opening Google..." : "Continue with Google"}
          </button>
        </div>
      ) : null}
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
          readOnly={inviteLocked}
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
          {busy ? "Creating account..." : inviteLocked ? "Join project" : "Create account"}
        </button>
      </form>
    </AuthCard>
  );
}
