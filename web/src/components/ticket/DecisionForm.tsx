import { Plus, X } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import type { Decision, DecisionInput } from "../../api/types";
import { cleanAlternatives, DECISION_LIMITS } from "../../lib/decisions";
import { AutosizeTextarea } from "../common/AutosizeTextarea";
import { Spinner } from "../common/Spinner";

/** Props for {@link DecisionForm}. */
export interface DecisionFormProps {
  /** The decision being replaced, when this form supersedes one. */
  replacing: Decision | null;
  onSubmit: (input: DecisionInput) => Promise<boolean>;
  onCancel: () => void;
}

/** Records a person's decision: what, why, and the options rejected. */
export function DecisionForm({ replacing, onSubmit, onCancel }: DecisionFormProps) {
  const id = useId();
  const [summary, setSummary] = useState("");
  const [rationale, setRationale] = useState("");
  const [alternatives, setAlternatives] = useState<string[]>([""]);
  const [isBusy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const remaining = DECISION_LIMITS.summary - summary.length;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (summary.trim() === "") {
      setProblem("Write the decision in one sentence.");
      return;
    }
    setProblem(null);
    setBusy(true);
    const input: DecisionInput = { summary: summary.trim(), rationale: rationale.trim(), alternatives: cleanAlternatives(alternatives) };
    if (replacing !== null) {
      input.supersedes_id = replacing.id;
    }
    const ok = await onSubmit(input);
    setBusy(false);
    if (ok) {
      onCancel();
    }
  };

  const setAlternative = (index: number, value: string) => {
    setAlternatives((current) => current.map((a, i) => (i === index ? value : a)));
  };

  return (
    <form className="inline-form decision-form" onSubmit={(e) => void submit(e)} noValidate>
      {replacing !== null && (
        <p className="decision-form__replacing">
          Replaces: <span className="decision-form__replaced">{replacing.summary}</span>
        </p>
      )}
      <div className="field">
        <label className="field__label" htmlFor={`${id}-summary`}>Decision</label>
        <input
          id={`${id}-summary`}
          className="input"
          value={summary}
          maxLength={DECISION_LIMITS.summary}
          onChange={(e) => setSummary(e.target.value)}
          aria-describedby={`${id}-summary-count`}
          autoFocus
        />
        <p id={`${id}-summary-count`} className="field__hint">{remaining} characters left</p>
      </div>
      <div className="field">
        <label className="field__label" htmlFor={`${id}-rationale`}>Why (optional)</label>
        <AutosizeTextarea
          id={`${id}-rationale`}
          rows={2}
          value={rationale}
          maxLength={DECISION_LIMITS.rationale}
          onChange={(e) => setRationale(e.target.value)}
        />
      </div>
      <fieldset className="field decision-form__alternatives">
        <legend className="field__label">Rejected options (optional)</legend>
        {alternatives.map((alternative, index) => (
          <div key={index} className="decision-form__alternative">
            <label className="visually-hidden" htmlFor={`${id}-alt-${index}`}>Rejected option {index + 1}</label>
            <input
              id={`${id}-alt-${index}`}
              className="input"
              value={alternative}
              maxLength={DECISION_LIMITS.alternative}
              onChange={(e) => setAlternative(index, e.target.value)}
            />
            {alternatives.length > 1 && (
              <button
                type="button"
                className="icon-button icon-button--small"
                aria-label={`Remove rejected option ${index + 1}`}
                onClick={() => setAlternatives((current) => current.filter((_, i) => i !== index))}
              >
                <X size={16} aria-hidden="true" />
              </button>
            )}
          </div>
        ))}
        {alternatives.length < DECISION_LIMITS.alternatives && (
          <button type="button" className="button button--ghost button--small" onClick={() => setAlternatives((c) => [...c, ""])}>
            <Plus size={14} aria-hidden="true" />
            Add another
          </button>
        )}
      </fieldset>
      {problem !== null && <p className="form-error" role="alert">{problem}</p>}
      <div className="row-actions">
        <button type="button" className="button button--secondary" onClick={onCancel}>Cancel</button>
        <button type="submit" className="button button--primary" disabled={isBusy}>
          {isBusy && <Spinner label="Saving" />}
          {replacing !== null ? "Replace decision" : "Record decision"}
        </button>
      </div>
    </form>
  );
}
