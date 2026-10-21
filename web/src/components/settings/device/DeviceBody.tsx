import type { DeviceState } from "../../../lib/deviceFlow";
import { Spinner } from "../../common/Spinner";
import { ConfirmStep } from "./ConfirmStep";
import { DoneStep } from "./DoneStep";
import { RetryStep } from "./RetryStep";
import { WaitingStep } from "./WaitingStep";

/** Props for {@link DeviceBody}. */
export interface DeviceBodyProps {
  state: DeviceState;
  remainingMs: number;
  onConfirm: () => void;
  onRestart: () => void;
  onClose: () => void;
}

/** The Connect GitHub dialog's content for the current phase. */
export function DeviceBody({ state, remainingMs, onConfirm, onRestart, onClose }: DeviceBodyProps) {
  switch (state.phase) {
    case "starting":
      return <p className="device__status" role="status"><Spinner label="Asking GitHub for a code" /> Asking GitHub for a code…</p>;
    case "waiting":
      return <WaitingStep state={state} remainingMs={remainingMs} onCancel={onClose} />;
    case "confirm":
    case "confirming":
      return <ConfirmStep state={state} onConfirm={onConfirm} onDecline={onClose} />;
    case "syncing":
      return (
        <p className="device__status" role="status">
          <Spinner label="Syncing" /> Connected as @{state.connection?.account ?? state.login}. Syncing your repos…
        </p>
      );
    case "done":
      return <DoneStep state={state} onClose={onClose} />;
    case "expired":
      return <RetryStep message="The code expired before it was entered." actionLabel="Get a new code" onRetry={onRestart} onClose={onClose} />;
    case "denied":
      return (
        <RetryStep
          message={state.message !== "" ? state.message : "GitHub sign-in was cancelled."}
          actionLabel="Try again"
          onRetry={onRestart}
          onClose={onClose}
        />
      );
    case "failed":
      return <RetryStep message={`Couldn't start GitHub sign-in: ${state.message}`} actionLabel="Try again" onRetry={onRestart} onClose={onClose} />;
  }
}
