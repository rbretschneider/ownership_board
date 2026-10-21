import { CircleCheck } from "lucide-react";
import type { DeviceState } from "../../../lib/deviceFlow";

/** The flow finished; shows how the first sync went. */
export function DoneStep({ state, onClose }: { state: DeviceState; onClose: () => void }) {
  const connection = state.connection;
  const error = connection?.last_sync?.error ?? "";
  const hasSynced = connection !== null && connection.last_sync !== null;
  return (
    <>
      <p className="device__status">
        <CircleCheck size={18} className="source__icon--ok" aria-hidden="true" />
        <span>Connected as @{connection?.account ?? state.login}.</span>
      </p>
      {hasSynced && error === "" && (
        <p>Synced {connection.repo_count} {connection.repo_count === 1 ? "repo" : "repos"}.</p>
      )}
      {error !== "" && <p className="form-error" role="alert">The first sync failed: {error} Use Sync now in Settings to retry.</p>}
      {!hasSynced && <p className="muted">The first sync is still running. Repos appear in Settings when it finishes.</p>}
      <div className="row-actions">
        <button type="button" className="button button--primary" onClick={onClose} data-autofocus>Done</button>
      </div>
    </>
  );
}
