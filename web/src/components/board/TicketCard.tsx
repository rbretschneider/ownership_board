import type { MouseEvent, PointerEvent } from "react";
import type { Repo, TicketSummary } from "../../api/types";
import { CARD_ATTR } from "../../lib/dropTargets";
import { ticketHash } from "../../lib/hashRoute";
import { TicketCardContent } from "./TicketCardContent";

/** Props for {@link TicketCard}. */
export interface TicketCardProps {
  ticket: TicketSummary;
  repo: Repo | undefined;
  /** This card is the one being dragged (rendered as a faded source). */
  isDragSource: boolean;
  onPointerDown: (event: PointerEvent<HTMLElement>, ticketId: number) => void;
  /** Return true to cancel navigation (the click ended a drag). */
  shouldIgnoreClick: () => boolean;
}

/** A board card. It is a link to the ticket so it works with keyboard and middle-click. */
export function TicketCard({ ticket, repo, isDragSource, onPointerDown, shouldIgnoreClick }: TicketCardProps) {
  const onClick = (event: MouseEvent<HTMLAnchorElement>) => {
    if (shouldIgnoreClick()) {
      event.preventDefault();
    }
  };
  return (
    <a
      href={ticketHash(ticket.id)}
      className={`card${isDragSource ? " card--source" : ""}${ticket.owner === "human" ? " card--yours" : ""}`}
      {...{ [CARD_ATTR]: ticket.id }}
      draggable={false}
      onPointerDown={(event) => onPointerDown(event, ticket.id)}
      onClick={onClick}
    >
      <TicketCardContent ticket={ticket} repo={repo} />
    </a>
  );
}
