import type { PointerEvent } from "react";
import type { Kind, Lane, LaneId, Repo, Stage, TicketSummary } from "../../api/types";
import type { DropTarget } from "../../lib/dropTargets";

/** What both board layouts (desktop columns, mobile single lane) receive. */
export interface BoardViewProps {
  lanes: Lane[];
  stages: Stage[];
  ticketsByLane: Map<LaneId, TicketSummary[]>;
  reposById: Map<number, Repo>;
  draggedId: number | null;
  target: DropTarget | null;
  onPointerDown: (event: PointerEvent<HTMLElement>, ticketId: number) => void;
  shouldIgnoreClick: () => boolean;
  onQuickAdd?: (kind: Kind) => void;
}
