import { LogIn } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useApi } from "../../api/ApiContext";
import type { AuthStatus } from "../../api/types";
import { OIDC_START_URL, SSO_ERROR_MESSAGES } from "../../lib/constants";
import { Spinner } from "../common/Spinner";
import { AuthLayout } from "./AuthLayout";

/** Props for {@link LoginForm}. */
export interface LoginFormProps {
  status: AuthStatus;
  /** Error code from `#/login?error=<code>` after an SSO callback. */
  ssoError: string | null;
  onSignedIn: () => void;
}

/** Password sign-in and/or the SSO button, depending on server config. */
export function LoginForm({ status, ssoError, onSignedIn }: LoginFormProps) {
  const api = useApi();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(ssoError === null ? null : SSO_ERROR_MESSAGES[ssoError] ?? SSO_ERROR_MESSAGES["sso_failed"] ?? null);
  const [isBusy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (email.trim() === "" || password === "") {
      setError("Enter your email and password.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.login({ email: email.trim(), password });
      onSignedIn();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setBusy(false);
    }
  };

  return (
    <AuthLayout title="Sign in" subtitle="Your private board for AI-driven work.">
      {error !== null && <p className="form-error" role="alert">{error}</p>}
      {status.password_login && (
        <form className="auth__form" onSubmit={(e) => void submit(e)} noValidate>
          <div className="field">
            <label className="field__label" htmlFor="login-email">Email</label>
            <input id="login-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="username" autoFocus />
          </div>
          <div className="field">
            <label className="field__label" htmlFor="login-password">Password</label>
            <input id="login-password" className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </div>
          <button type="submit" className="button button--primary button--block" disabled={isBusy}>
            {isBusy && <Spinner label="Signing in" />}
            {isBusy ? "Signing in…" : "Sign in"}
          </button>
        </form>
      )}
      {status.password_login && status.oidc.enabled && <div className="auth__divider"><span>or</span></div>}
      {status.oidc.enabled && (
        <a className="button button--secondary button--block" href={OIDC_START_URL}>
          <LogIn size={18} aria-hidden="true" />
          {status.oidc.label}
        </a>
      )}
      {!status.password_login && !status.oidc.enabled && (
        <p className="muted">No sign-in method is enabled on this server. Check its configuration.</p>
      )}
    </AuthLayout>
  );
}
