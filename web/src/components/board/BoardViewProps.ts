import type { PointerEvent } from "react";
import type { Lane, LaneId, Repo, TicketSummary } from "../../api/types";
import type { DropTarget } from "../../lib/dropTargets";

/** What both board layouts (desktop columns, mobile single lane) receive. */
export interface BoardViewProps {
  lanes: Lane[];
  ticketsByLane: Map<LaneId, TicketSummary[]>;
  reposById: Map<number, Repo>;
  draggedId: number | null;
  target: DropTarget | null;
  onPointerDown: (event: PointerEvent<HTMLElement>, ticketId: number) => void;
  shouldIgnoreClick: () => boolean;
}
