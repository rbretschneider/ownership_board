import { useId, useState, type FormEvent } from "react";
import type { WorkLinkInput, WorkLinkKind } from "../../api/types";
import { toWorkLinkInput } from "../../lib/workLinks";
import { Spinner } from "../common/Spinner";

const KINDS: readonly { kind: WorkLinkKind; label: string }[] = [
  { kind: "pr", label: "Pull request" },
  { kind: "commit", label: "Commit" },
];

/** Adds a PR or commit to a ticket's work links. */
export function WorkLinkForm({ onSubmit, onCancel }: { onSubmit: (input: WorkLinkInput) => Promise<boolean>; onCancel: () => void }) {
  const id = useId();
  const [kind, setKind] = useState<WorkLinkKind>("pr");
  const [url, setUrl] = useState("");
  const [sha, setSha] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const result = toWorkLinkInput({ kind, url, sha });
    if ("problem" in result) {
      setProblem(result.problem);
      return;
    }
    setProblem(null);
    setBusy(true);
    const ok = await onSubmit(result.input);
    setBusy(false);
    if (ok) {
      onCancel();
    }
  };

  return (
    <form className="inline-form" onSubmit={(e) => void submit(e)} noValidate>
      <fieldset className="segmented" aria-label="Link type">
        {KINDS.map((option) => (
          <label key={option.kind} className={`segmented__option${kind === option.kind ? " segmented__option--active" : ""}`}>
            <input
              type="radio"
              className="visually-hidden"
              name={`${id}-kind`}
              value={option.kind}
              checked={kind === option.kind}
              onChange={() => setKind(option.kind)}
            />
            {option.label}
          </label>
        ))}
      </fieldset>
      {kind === "commit" && (
        <div className="field">
          <label className="field__label" htmlFor={`${id}-sha`}>Commit sha</label>
          <input id={`${id}-sha`} className="input input--mono" value={sha} onChange={(e) => setSha(e.target.value)} spellCheck={false} autoComplete="off" />
        </div>
      )}
      <div className="field">
        <label className="field__label" htmlFor={`${id}-url`}>{kind === "pr" ? "Pull request URL" : "Commit URL (optional)"}</label>
        <input id={`${id}-url`} className="input" type="url" inputMode="url" value={url} onChange={(e) => setUrl(e.target.value)} />
        <p className="field__hint">It must be on the same host as the ticket's repo.</p>
      </div>
      {problem !== null && <p className="form-error" role="alert">{problem}</p>}
      <div className="row-actions">
        <button type="button" className="button button--secondary" onClick={onCancel}>Cancel</button>
        <button type="submit" className="button button--primary" disabled={isBusy}>
          {isBusy && <Spinner label="Adding" />}
          Add link
        </button>
      </div>
    </form>
  );
}
