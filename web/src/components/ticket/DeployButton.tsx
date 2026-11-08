import { useRef, useState } from "react";
import type { Repo, TicketSummary } from "../../api/types";
import { useCurrentUser } from "../../hooks/useCurrentUser";
import { canRequestDeploy } from "../../lib/deployment";
import { Spinner } from "../common/Spinner";

/** Shared deployment action; callers report errors and accept authoritative updates. */
export function DeployButton({ticket, repo, onDeploy}: {
  ticket: TicketSummary;
  repo: Repo | undefined;
  onDeploy: () => Promise<boolean>;
}) {
  const user = useCurrentUser();
  const pending = useRef(false);
  const [isPending, setPending] = useState(false);
  if (!canRequestDeploy(ticket, repo, user)) return null;
  const deploy = async () => {
    if (pending.current) return;
    pending.current = true;
    setPending(true);
    try {
      await onDeploy();
    } finally {
      pending.current = false;
      setPending(false);
    }
  };
  return <button type="button" className="button button--primary" disabled={isPending} onClick={() => void deploy()}>
    {isPending && <Spinner label="Requesting deployment" />}
    Deploy now
  </button>;
}
