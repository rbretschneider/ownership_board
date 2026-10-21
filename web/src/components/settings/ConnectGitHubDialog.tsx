import { useCallback } from "react";
import { useApi } from "../../api/ApiContext";
import type { Connection } from "../../api/types";
import { useGitHubDeviceFlow } from "../../hooks/useGitHubDeviceFlow";
import { Overlay } from "../common/Overlay";
import { useToast } from "../toast/ToastContext";
import { DeviceBody } from "./device/DeviceBody";

/** Props for {@link ConnectGitHubDialog}. */
export interface ConnectGitHubDialogProps {
  onClose: () => void;
  /** Called once the connection is stored, and again when its first sync lands. */
  onConnected: (connection: Connection) => void;
}

/**
 * Connects GitHub with the device flow: shows a code, waits while the person
 * enters it on github.com, asks them to confirm the account GitHub reports,
 * then waits for the first repo sync. Escape or Cancel abandons the flow.
 */
export function ConnectGitHubDialog({ onClose, onConnected }: ConnectGitHubDialogProps) {
  const api = useApi();
  const toast = useToast();
  const flow = useGitHubDeviceFlow(api, {
    onConnected,
    onFirstSync: (connection) => {
      const error = connection.last_sync?.error ?? "";
      if (error !== "") {
        toast.error(`GitHub connected, but the first sync failed: ${error}`);
        return;
      }
      toast.success(`GitHub connected · synced ${connection.repo_count} ${connection.repo_count === 1 ? "repo" : "repos"}`);
    },
  });
  const { cancel } = flow;

  const close = useCallback(() => {
    cancel();
    onClose();
  }, [cancel, onClose]);

  return (
    <Overlay variant="dialog" label="Connect GitHub" onClose={close}>
      <div className="sheet">
        <header className="sheet__header">
          <h2 className="sheet__title">Connect GitHub</h2>
        </header>
        <div className="sheet__body device">
          <DeviceBody
            state={flow.state}
            remainingMs={flow.remainingMs}
            onConfirm={() => void flow.confirm()}
            onRestart={flow.restart}
            onClose={close}
          />
        </div>
      </div>
    </Overlay>
  );
}
