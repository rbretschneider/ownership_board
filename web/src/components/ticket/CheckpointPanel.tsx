import { Check, Code, FileCheck, RotateCw, Undo2 } from "lucide-react";
import { useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Checkpoint, TicketDetail } from "../../api/types";
import {
  checkpointDocument,
  isStaleConflict,
  requestChangesError,
  STALE_CHECKPOINT_MESSAGE,
} from "../../lib/stages";
import { AutosizeTextarea } from "../common/AutosizeTextarea";
import { Markdown } from "../common/Markdown";
import { Spinner } from "../common/Spinner";

const HEADINGS: Record<Checkpoint, string> = { spec: "Approve the spec", design: "Approve the design" };

/** Props for {@link CheckpointPanel}. */
export interface CheckpointPanelProps {
  ticket: TicketDetail;
  checkpoint: Checkpoint;
  /** Applies a successful result (updates the sheet and the board). */
  onResolved: (ticket: TicketDetail, message: string) => void;
  /** Reloads the ticket after a 409. */
  onReload: () => void;
}

/**
 * The owner's checkpoint: the document as shown, then Approve or Request
 * changes. Both send the revision rendered here, so an edit in between is a
 * 409 instead of approving text the owner never saw.
 */
export function CheckpointPanel({ ticket, checkpoint, onResolved, onReload }: CheckpointPanelProps) {
  const api = useApi();
  const id = useId();
  const { text, revision } = checkpointDocument(ticket, checkpoint);
  const [comment, setComment] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);
  const [isStale, setStale] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isSource, setSource] = useState(false);
  const noun = checkpoint === "spec" ? "spec" : "design";

  const submit = async (action: () => Promise<TicketDetail>, message: string) => {
    setBusy(true);
    setError(null);
    try {
      onResolved(await action(), message);
    } catch (cause) {
      if (isStaleConflict(cause)) {
        setStale(true);
      } else {
        setError(cause instanceof Error ? cause.message : String(cause));
      }
      setBusy(false);
    }
  };

  const approve = () => submit(() => api.approve(ticket.id, { checkpoint, revision }), `Approved the ${noun}`);
  const requestChanges = () => {
    if (comment === null) {
      return;
    }
    const invalid = requestChangesError(comment);
    if (invalid !== null) {
      setError(invalid);
      return;
    }
    void submit(
      () => api.requestChanges(ticket.id, { checkpoint, revision, comment: comment.trim() }),
      `Sent the ${noun} back to the AI`,
    );
  };

  return (
    <section className="checkpoint" aria-labelledby={`${id}-title`}>
      <header className="checkpoint__header">
        <FileCheck size={20} aria-hidden="true" />
        <h3 id={`${id}-title`} className="checkpoint__title">{HEADINGS[checkpoint]}</h3>
        <span className="checkpoint__revision">revision {revision}</span>
        <button
          type="button"
          className="button button--ghost checkpoint__source"
          aria-pressed={isSource}
          onClick={() => setSource(!isSource)}
        >
          <Code size={16} aria-hidden="true" />
          {isSource ? "Formatted" : "Source"}
        </button>
      </header>
      <div className="checkpoint__document">
        {text.trim() === "" ? (
          <p className="muted">The {noun} is empty.</p>
        ) : isSource ? (
          <pre className="checkpoint__raw">{text}</pre>
        ) : (
          <Markdown source={text} />
        )}
      </div>
      {isStale && (
        <div className="checkpoint__stale" role="alert">
          <p>{STALE_CHECKPOINT_MESSAGE}</p>
          <button type="button" className="button button--secondary" onClick={onReload}>
            <RotateCw size={16} aria-hidden="true" />
            Reload
          </button>
        </div>
      )}
      {error !== null && <p className="form-error" role="alert">{error}</p>}
      {!isStale && comment === null && (
        <div className="row-actions">
          <button type="button" className="button button--secondary" onClick={() => setComment("")} disabled={isBusy}>
            <Undo2 size={16} aria-hidden="true" />
            Request changes
          </button>
          <button type="button" className="button button--primary" onClick={() => void approve()} disabled={isBusy}>
            {isBusy ? <Spinner label="Approving" /> : <Check size={16} aria-hidden="true" />}
            Approve
          </button>
        </div>
      )}
      {!isStale && comment !== null && (
        <div className="field">
          <label className="field__label" htmlFor={`${id}-comment`}>What should change? (required)</label>
          <AutosizeTextarea
            id={`${id}-comment`}
            value={comment}
            onChange={(e) => {
              setComment(e.target.value);
              setError(null);
            }}
            aria-required="true"
            autoFocus
          />
          <div className="row-actions">
            <button type="button" className="button button--secondary" onClick={() => { setComment(null); setError(null); }} disabled={isBusy}>
              Cancel
            </button>
            <button type="button" className="button button--primary" onClick={requestChanges} disabled={isBusy}>
              {isBusy && <Spinner label="Sending" />}
              Send back
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
