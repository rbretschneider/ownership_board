import { useState, type FormEvent } from "react";
import { useApi } from "../../api/ApiContext";
import { MIN_PASSWORD_LENGTH } from "../../lib/constants";
import { Spinner } from "../common/Spinner";
import { AuthLayout } from "./AuthLayout";

/** First-run wizard: creates the admin account. */
export function SetupForm({ onDone }: { onDone: () => void }) {
  const api = useApi();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const problem = validate({ name, email, password, confirm });
    if (problem !== null) {
      setError(problem);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.setup({ name: name.trim(), email: email.trim(), password });
      onDone();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setBusy(false);
    }
  };

  return (
    <AuthLayout title="Set up your board" subtitle="Create the admin account. You can add single sign-on later.">
      <form className="auth__form" onSubmit={(e) => void submit(e)} noValidate>
        <div className="field">
          <label className="field__label" htmlFor="setup-name">Name</label>
          <input id="setup-name" className="input" value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" autoFocus />
        </div>
        <div className="field">
          <label className="field__label" htmlFor="setup-email">Email</label>
          <input id="setup-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" />
        </div>
        <div className="field">
          <label className="field__label" htmlFor="setup-password">Password</label>
          <input
            id="setup-password"
            className="input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            aria-describedby="setup-password-hint"
          />
          <p id="setup-password-hint" className="field__hint">At least {MIN_PASSWORD_LENGTH} characters.</p>
        </div>
        <div className="field">
          <label className="field__label" htmlFor="setup-confirm">Confirm password</label>
          <input id="setup-confirm" className="input" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" />
        </div>
        {error !== null && <p className="form-error" role="alert">{error}</p>}
        <button type="submit" className="button button--primary button--block" disabled={isBusy}>
          {isBusy && <Spinner label="Creating account" />}
          {isBusy ? "Creating account…" : "Create account"}
        </button>
      </form>
    </AuthLayout>
  );
}

function validate(input: { name: string; email: string; password: string; confirm: string }): string | null {
  if (input.name.trim() === "") {
    return "Enter your name.";
  }
  if (!input.email.includes("@")) {
    return "Enter a valid email address.";
  }
  if (input.password.length < MIN_PASSWORD_LENGTH) {
    return `Use a password of at least ${MIN_PASSWORD_LENGTH} characters.`;
  }
  if (input.password !== input.confirm) {
    return "The passwords don't match. Re-enter the confirmation.";
  }
  return null;
}
