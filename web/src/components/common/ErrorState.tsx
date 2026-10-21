import { RefreshCw } from "lucide-react";

/** A failure message with a retry path. */
export function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="empty-state" role="alert">
      <p>{message}</p>
      <button type="button" className="button button--primary" onClick={onRetry}>
        <RefreshCw size={16} aria-hidden="true" />
        Try again
      </button>
    </div>
  );
}
