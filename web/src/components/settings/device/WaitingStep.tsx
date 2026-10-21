import { ExternalLink } from "lucide-react";
import { formatCountdown, GITHUB_DEVICE_URL, type DeviceState } from "../../../lib/deviceFlow";
import { CopyButton } from "../../common/CopyButton";
import { Spinner } from "../../common/Spinner";

/**
 * Shows the code and waits while the person enters it on GitHub. The link is
 * always GitHub's own device page, never a URL taken from a response.
 */
export function WaitingStep({ state, remainingMs, onCancel }: { state: DeviceState; remainingMs: number; onCancel: () => void }) {
  const code = state.flow?.user_code ?? "";
  return (
    <>
      <p>Enter this code on GitHub:</p>
      <div className="device__code-row">
        <code className="device__code">{code}</code>
        <CopyButton text={code} label="Copy the code" />
      </div>
      <a className="button button--primary button--block" href={GITHUB_DEVICE_URL} target="_blank" rel="noopener noreferrer" data-autofocus>
        <ExternalLink size={16} aria-hidden="true" />
        Open GitHub
      </a>
      <p className="device__status" role="status">
        <Spinner label="Waiting" />
        <span>Waiting for you to enter the code on GitHub… <span className="muted">({formatCountdown(remainingMs)} left)</span></span>
      </p>
      {state.message !== "" && <p className="form-error" role="alert">{state.message} Still trying.</p>}
      <div className="row-actions">
        <button type="button" className="button button--secondary" onClick={onCancel}>Cancel</button>
      </div>
    </>
  );
}
