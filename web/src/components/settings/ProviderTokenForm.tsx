import { useState, type FormEvent } from "react";
import type { Connection } from "../../api/types";
import { isPlainHttp, submitSecret } from "../../lib/secretField";
import { Spinner } from "../common/Spinner";

/** The non-secret field shown above the token (base URL or email). */
export interface AccountField {
  label: string;
  type: "url" | "email";
  isRequired: boolean;
  placeholder: string;
  /** Shown under the field. */
  hint?: string;
  /** Warn about plain-HTTP addresses (Forgejo base URL). */
  isUrlWarned?: boolean;
}

/** Props for {@link ProviderTokenForm}. */
export interface ProviderTokenFormProps {
  /** Unique prefix for element ids. */
  idPrefix: string;
  title: string;
  field: AccountField;
  tokenHint: string;
  /** Pre-fill for a reconnect. */
  initialAccount?: string;
  submitLabel: string;
  onSubmit: (account: string, token: string) => Promise<Connection>;
  onConnected: (connection: Connection) => void;
  onCancel?: () => void;
}

/**
 * Connects a token-based provider. The token field is a password field that is
 * emptied as soon as the request is sent, whether it succeeds or fails.
 */
export function ProviderTokenForm(props: ProviderTokenFormProps) {
  const { idPrefix, field } = props;
  const [account, setAccount] = useState(props.initialAccount ?? "");
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);
  const accountId = `${idPrefix}-account`;
  const tokenId = `${idPrefix}-token`;
  const errorId = `${idPrefix}-error`;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const problem = validate(account, token, field);
    if (problem !== null) {
      setError(problem);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const connection = await submitSecret(token, () => setToken(""), (secret) => props.onSubmit(account.trim(), secret));
      props.onConnected(connection);
    } catch (cause) {
      setError(`${cause instanceof Error ? cause.message : String(cause)} Check the details and try again.`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="inline-form" onSubmit={(e) => void submit(e)} noValidate aria-labelledby={`${idPrefix}-title`}>
      <h4 id={`${idPrefix}-title`} className="provider-form__title">{props.title}</h4>
      <div className="field">
        <label className="field__label" htmlFor={accountId}>
          {field.label}{field.isRequired ? "" : " (optional)"}
        </label>
        <input
          id={accountId}
          className="input"
          type={field.type}
          inputMode={field.type}
          value={account}
          onChange={(e) => setAccount(e.target.value)}
          placeholder={field.placeholder}
          autoComplete="off"
          spellCheck={false}
          aria-describedby={field.hint !== undefined ? `${accountId}-hint` : undefined}
        />
        {field.hint !== undefined && <p id={`${accountId}-hint`} className="field__hint">{field.hint}</p>}
        {field.isUrlWarned === true && isPlainHttp(account) && (
          <p className="field__warning" role="note">
            This address uses plain http://, so the token crosses the network unencrypted.
          </p>
        )}
      </div>
      <div className="field">
        <label className="field__label" htmlFor={tokenId}>Token</label>
        <input
          id={tokenId}
          className="input"
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          autoComplete="off"
          spellCheck={false}
          aria-describedby={`${tokenId}-hint`}
          aria-invalid={error !== null}
        />
        <p id={`${tokenId}-hint`} className="field__hint">{props.tokenHint}</p>
      </div>
      {error !== null && <p id={errorId} className="form-error" role="alert">{error}</p>}
      <div className="row-actions">
        {props.onCancel !== undefined && (
          <button type="button" className="button button--secondary" onClick={props.onCancel}>Cancel</button>
        )}
        <button type="submit" className="button button--primary" disabled={isBusy}>
          {isBusy && <Spinner label="Checking the token" />}
          {isBusy ? "Checking…" : props.submitLabel}
        </button>
      </div>
    </form>
  );
}

function validate(account: string, token: string, field: AccountField): string | null {
  const value = account.trim();
  if (field.isRequired && value === "") {
    return `Enter the ${field.label.toLowerCase()}.`;
  }
  if (field.type === "url" && value !== "" && !/^https?:\/\/[^\s/]+/i.test(value)) {
    return "Use a full address such as https://git.example.com.";
  }
  if (field.type === "email" && value !== "" && !value.includes("@")) {
    return "Enter the account email, or leave it empty for an access token.";
  }
  if (token.trim() === "") {
    return "Paste the token.";
  }
  return null;
}
