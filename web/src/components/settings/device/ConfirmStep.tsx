import { TriangleAlert } from "lucide-react";
import type { DeviceState } from "../../../lib/deviceFlow";
import { Spinner } from "../../common/Spinner";

/**
 * Asks the person to confirm the account GitHub reported. Nothing is stored
 * until they do, so someone else's code entered by mistake connects nothing.
 */
export function ConfirmStep({ state, onConfirm, onDecline }: { state: DeviceState; onConfirm: () => void; onDecline: () => void }) {
  const isBusy = state.phase === "confirming";
  return (
    <>
      <p className="device__question">
        GitHub says you are <strong>@{state.login}</strong>. Connect this account?
      </p>
      {state.replaces !== "" && (
        <p className="field__warning" role="note">
          <TriangleAlert size={16} aria-hidden="true" /> This replaces the connected account @{state.replaces}.
        </p>
      )}
      {state.message !== "" && <p className="form-error" role="alert">{state.message} Try again, or choose Not me.</p>}
      <div className="row-actions">
        <button type="button" className="button button--secondary" onClick={onDecline} disabled={isBusy}>Not me</button>
        <button type="button" className="button button--primary" onClick={onConfirm} disabled={isBusy} data-autofocus>
          {isBusy && <Spinner label="Connecting" />}
          Connect @{state.login}
        </button>
      </div>
    </>
  );
}
