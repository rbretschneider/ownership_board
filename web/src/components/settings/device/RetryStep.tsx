/** A failed state that always offers a way forward: retry, or close. */
export function RetryStep({ message, actionLabel, onRetry, onClose }: {
  message: string;
  actionLabel: string;
  onRetry: () => void;
  onClose: () => void;
}) {
  return (
    <>
      <p className="form-error" role="alert">{message}</p>
      <div className="row-actions">
        <button type="button" className="button button--secondary" onClick={onClose}>Close</button>
        <button type="button" className="button button--primary" onClick={onRetry} data-autofocus>{actionLabel}</button>
      </div>
    </>
  );
}
