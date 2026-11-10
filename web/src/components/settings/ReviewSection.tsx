import { useEffect, useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BoardSettings, ReviewBy } from "../../api/types";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { useToast } from "../toast/ToastContext";

type ReviewSettings = Pick<BoardSettings, "review_by" | "review_fresh_context" | "review_adversarial">;

/** The first two choices are always offered; the rest are the agent accounts. */
export const REVIEWERS: readonly { value: ReviewBy; label: string }[] = [
  { value: "other_model", label: "Any model except the engineer's" },
  { value: "same_model", label: "Same model as the engineer" },
  { value: "claude", label: "Claude account" },
  { value: "codex", label: "Codex account" },
];

/**
 * Board-wide review policy: who reviews an agent's finished work before it is
 * published, and how. Which findings block publication is set per repo.
 */
export function ReviewSection() {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [settings, setSettings] = useState<ReviewSettings | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    api.settings().then(
      (s) => { if (active) setSettings(pick(s)); },
      (cause) => { if (active) setError(String(cause)); },
    );
    return () => { active = false; };
  }, [api]);

  const save = async (patch: Partial<ReviewSettings>) => {
    if (!settings) return;
    const before = settings;
    setSettings({ ...settings, ...patch });
    setBusy(true);
    try {
      setSettings(pick(await api.editSettings(patch)));
    } catch (cause) {
      setSettings(before);
      toast.error(`Couldn't save review settings: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-section" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="settings-section__title">Reviews</h2>
      {error && <ErrorState message={error} onRetry={() => window.location.reload()} />}
      {!settings && !error && <LoadingState label="Loading review settings" />}
      {settings && (
        <>
          <div className="field">
            <label className="field__label" htmlFor={`${id}-by`}>Who reviews</label>
            <select className="input select" id={`${id}-by`} value={settings.review_by} disabled={busy}
              aria-describedby={`${id}-by-hint`}
              onChange={(e) => void save({ review_by: e.target.value as ReviewBy })}>
              {REVIEWERS.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
            </select>
            <p className="field__hint" id={`${id}-by-hint`}>
              {"Any model except the engineer's prefers the other account and uses a different model on the same account when the other one is out of weekly usage. A pinned account that is out of usage falls back the same way."}
            </p>
          </div>
          <div className="field">
            <label className="switch" htmlFor={`${id}-fresh`}>
              <input id={`${id}-fresh`} type="checkbox" role="switch" className="switch__input" checked={settings.review_fresh_context}
                disabled={busy} onChange={(e) => void save({ review_fresh_context: e.target.checked })} />
              <span className="switch__track" aria-hidden="true" />
              <span className="switch__label">Clear of context</span>
            </label>
            <p className="muted small">On: the reviewer sees only the spec, design and code. Off: it also gets the engineer's handoff (summary and decisions).</p>
          </div>
          <div className="field">
            <label className="switch" htmlFor={`${id}-adversarial`}>
              <input id={`${id}-adversarial`} type="checkbox" role="switch" className="switch__input" checked={settings.review_adversarial}
                disabled={busy} onChange={(e) => void save({ review_adversarial: e.target.checked })} />
              <span className="switch__track" aria-hidden="true" />
              <span className="switch__label">Adversarial</span>
            </label>
            <p className="muted small">On: the reviewer hunts for defects and approves only when none remain. Off: it checks the work against the spec and approves unless something is actually wrong.</p>
          </div>
          <p className="muted small">Which findings block publication (critical only, must-fix, should-fix or nits too) is set per repository under Repositories.</p>
        </>
      )}
    </section>
  );
}

function pick(s: BoardSettings): ReviewSettings {
  return {
    review_by: s.review_by ?? "other_model",
    review_fresh_context: s.review_fresh_context ?? true,
    review_adversarial: s.review_adversarial ?? true,
  };
}
