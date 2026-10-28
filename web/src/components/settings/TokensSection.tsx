import { KeyRound, Plus, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useApi } from "../../api/ApiContext";
import type { CreatedToken, Token } from "../../api/types";
import { relativeTime } from "../../lib/relativeTime";
import { ConfirmDialog } from "../common/ConfirmDialog";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { Spinner } from "../common/Spinner";
import { useToast } from "../toast/ToastContext";
import { TokenSecret } from "./TokenSecret";

/** Agent access tokens: list, create (secret shown once), revoke. */
export function TokensSection() {
  const api = useApi();
  const toast = useToast();
  const [tokens, setTokens] = useState<Token[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [isDelegate, setDelegate] = useState(false);
  const [isCreating, setCreating] = useState(false);
  const [created, setCreated] = useState<CreatedToken | null>(null);
  const [revoking, setRevoking] = useState<Token | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      setTokens(await api.tokens());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [api]);

  useEffect(() => {
    void load();
  }, [load]);

  const create = async (event: FormEvent) => {
    event.preventDefault();
    if (name.trim() === "") {
      return;
    }
    setCreating(true);
    try {
      const token = await api.createToken(name.trim(), isDelegate);
      setCreated(token);
      setName("");
      setDelegate(false);
      setTokens((current) => [...(current ?? []), token]);
    } catch (cause) {
      toast.error(`Couldn't create the token: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setCreating(false);
    }
  };

  const revoke = async (token: Token) => {
    await api.revokeToken(token.id);
    setTokens((current) => (current ?? []).filter((t) => t.id !== token.id));
    setRevoking(null);
    toast.success(`Revoked ${token.name}`);
  };

  return (
    <section className="settings-section" aria-labelledby="tokens-title">
      <h3 id="tokens-title" className="settings-section__title">Agent tokens</h3>
      <p className="muted">Agents (Claude Code, scripts) use these to claim work, ask questions and move cards.</p>
      {created !== null && <TokenSecret token={created} onDone={() => setCreated(null)} />}
      {error !== null && <ErrorState message={`Couldn't load tokens: ${error}`} onRetry={() => void load()} />}
      {error === null && tokens === null && <LoadingState label="Loading tokens" />}
      {tokens !== null && tokens.length > 0 && (
        <ul className="token-list">
          {tokens.map((token) => (
            <li key={token.id} className="token-row">
              <KeyRound size={16} aria-hidden="true" />
              <div className="token-row__text">
                <span className="token-row__name">
                  {token.name}
                  {token.is_delegate && <span className="token-row__badge">delegate</span>}
                </span>
                <span className="muted small">
                  <code>{token.prefix}…</code> · {token.last_used_at === null ? "never used" : `used ${relativeTime(token.last_used_at)}`}
                </span>
              </div>
              <button type="button" className="icon-button" aria-label={`Revoke ${token.name}`} onClick={() => setRevoking(token)}>
                <Trash2 size={16} aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <form className="inline-form inline-form--row" onSubmit={(e) => void create(e)}>
        <div className="field field--grow">
          <label className="field__label" htmlFor="token-name">New token name</label>
          <input id="token-name" className="input" value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" />
        </div>
        <button type="submit" className="button button--secondary" disabled={isCreating || name.trim() === ""}>
          {isCreating ? <Spinner label="Creating" /> : <Plus size={16} aria-hidden="true" />}
          Create
        </button>
      </form>
      <label className="switch" htmlFor="token-delegate">
        <input
          id="token-delegate"
          type="checkbox"
          role="switch"
          className="switch__input"
          checked={isDelegate}
          onChange={(e) => setDelegate(e.target.checked)}
        />
        <span className="switch__track" aria-hidden="true" />
        <span className="switch__label">Delegate: may file autonomous tickets</span>
      </label>
      {isDelegate && (
        <p className="muted small">
          An autonomous ticket skips every checkpoint and question and goes straight to a pull request. Give this
          token only to the chat where you agree specs (for example desktop Claude), never to the runner.
        </p>
      )}
      {revoking !== null && (
        <ConfirmDialog
          title={`Revoke ${revoking.name}?`}
          message="Any agent using this token loses access immediately."
          confirmLabel="Revoke"
          onConfirm={() => revoke(revoking)}
          onCancel={() => setRevoking(null)}
        />
      )}
    </section>
  );
}
