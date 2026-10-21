import { useState } from "react";
import { Overlay } from "./Overlay";
import { Spinner } from "./Spinner";

/** Props for {@link ConfirmDialog}. */
export interface ConfirmDialogProps {
  title: string;
  message: string;
  confirmLabel: string;
  /** Performs the action; a rejection keeps the dialog open and shows the error. */
  onConfirm: () => Promise<void>;
  onCancel: () => void;
}

/** Asks before a destructive action. */
export function ConfirmDialog({ title, message, confirmLabel, onConfirm, onCancel }: ConfirmDialogProps) {
  const [isBusy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const confirm = async () => {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setBusy(false);
    }
  };

  return (
    <Overlay variant="alert" label={title} onClose={onCancel} isAlert>
      <div className="confirm">
        <h2 className="confirm__title">{title}</h2>
        <p className="confirm__message">{message}</p>
        {error !== null && <p className="form-error" role="alert">{error}</p>}
        <div className="confirm__actions">
          <button type="button" className="button button--secondary" onClick={onCancel} data-autofocus>
            Cancel
          </button>
          <button type="button" className="button button--danger" onClick={() => void confirm()} disabled={isBusy}>
            {isBusy ? <Spinner label="Working" /> : null}
            {confirmLabel}
          </button>
        </div>
      </div>
    </Overlay>
  );
}
