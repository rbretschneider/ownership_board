import { useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Repo } from "../../api/types";
import { useToast } from "../toast/ToastContext";

const MAX = 50;

/** Returns a message when draft is not a whole number from 0 to 50. */
function validate(draft: string): string | null {
  const text = draft.trim();
  if (!/^\d+$/.test(text) || Number(text) > MAX) {
    return `Enter a whole number from 0 to ${MAX} (0 turns the check off).`;
  }
  return null;
}

/**
 * How many merges pass between architecture checks of a repo. Saves on blur
 * or Enter, never per keystroke, and reverts to the saved value if refused.
 */
export function RepoHealthCheckField({ repo, onUpdated }: { repo: Repo; onUpdated: (repo: Repo) => void }) {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [draft, setDraft] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const shown = draft ?? String(repo.health_check_every);

  const save = async () => {
    if (draft === null || busy) return;
    const problem = validate(draft);
    if (problem !== null) {
      setError(problem);
      return;
    }
    const next = Number(draft.trim());
    if (next === repo.health_check_every) {
      setDraft(null);
      setError(null);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      onUpdated(await api.editRepo(repo.id, { health_check_every: next }));
      setDraft(null);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause);
      setDraft(null);
      setError(message);
      toast.error(`Couldn't change the architecture check for ${repo.full_name}: ${message}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="repo-health-check">
      <label htmlFor={id}>Architecture check every N merges (0 = off)</label>
      <input
        id={id}
        className="input"
        type="number"
        min={0}
        max={MAX}
        step={1}
        value={shown}
        disabled={busy}
        aria-invalid={error !== null}
        aria-describedby={error !== null ? `${id}-error` : undefined}
        onChange={(e) => {
          setDraft(e.target.value);
          setError(null);
        }}
        onBlur={() => void save()}
        onKeyDown={(e) => {
          if (e.key === "Enter") void save();
        }}
      />
      {error !== null && (
        <span id={`${id}-error`} role="alert" className="field-error small">{error}</span>
      )}
    </div>
  );
}
