import { useApi } from "../../api/ApiContext";
import { useToast } from "../toast/ToastContext";
import { DeployButton } from "../ticket/DeployButton";
import type { MouseEvent, PointerEvent } from "react";
import type { Repo, Stage, TicketSummary } from "../../api/types";
import { useLeaseClock } from "../../hooks/useLeaseClock";
import { cardTone } from "../../lib/activity";
import { CARD_ATTR } from "../../lib/dropTargets";
import { ticketHash } from "../../lib/hashRoute";
import { TicketCardContent } from "./TicketCardContent";

/** Props for {@link TicketCard}. */
export interface TicketCardProps {
  ticket: TicketSummary;
  stages: Stage[];
  repo: Repo | undefined;
  /** This card is the one being dragged (rendered as a faded source). */
  isDragSource: boolean;
  onPointerDown: (event: PointerEvent<HTMLElement>, ticketId: number) => void;
  /** Return true to cancel navigation (the click ended a drag). */
  shouldIgnoreClick: () => boolean;
}

/** A board card. It is a link to the ticket so it works with keyboard and middle-click. */
export function TicketCard({ ticket, stages, repo, isDragSource, onPointerDown, shouldIgnoreClick }: TicketCardProps) {
  const api = useApi();
  const toast = useToast();
  const now = useLeaseClock(ticket.lease?.expires_at);
  const onClick = (event: MouseEvent<HTMLAnchorElement>) => {
    if (shouldIgnoreClick()) {
      event.preventDefault();
    }
  };
  return (
    <div
      className={`card card--linked${isDragSource ? " card--source" : ""}${ticket.needs_owner ? " card--yours" : ""}${cardTone(ticket, now)}`}
      {...{ [CARD_ATTR]: ticket.id }}
      draggable={false}
      onPointerDown={(event) => onPointerDown(event, ticket.id)}
    >
      <a className="card__link" href={ticketHash(ticket.id)} draggable={false} onClick={onClick}>
        <TicketCardContent now={now} ticket={ticket} stages={stages} repo={repo} />
      </a>
      <div className="card__actions" onPointerDown={(event) => event.stopPropagation()}>
        <DeployButton ticket={ticket} repo={repo} onDeploy={async () => {
          if (shouldIgnoreClick()) return false;
          try {
            await api.deployTicket(ticket.id);
            return true;
          } catch (cause) {
            toast.error(cause instanceof Error ? cause.message : String(cause));
            return false;
          }
        }} />
      </div>
    </div>
  );
}
