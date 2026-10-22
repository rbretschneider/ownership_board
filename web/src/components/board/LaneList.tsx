import { Fragment, type PointerEvent } from "react";
import type { LaneId, Repo, Stage, TicketSummary } from "../../api/types";
import { LANE_EMPTY_HINTS } from "../../lib/constants";
import { DROP_LIST_ATTR, type DropTarget } from "../../lib/dropTargets";
import { TicketCard } from "./TicketCard";

/** Props for {@link LaneList}. */
export interface LaneListProps {
  lane: LaneId;
  tickets: TicketSummary[];
  stages: Stage[];
  reposById: Map<number, Repo>;
  draggedId: number | null;
  target: DropTarget | null;
  onPointerDown: (event: PointerEvent<HTMLElement>, ticketId: number) => void;
  shouldIgnoreClick: () => boolean;
}

/** The cards of one lane, plus the drop indicator while dragging. */
export function LaneList({ lane, tickets, stages, reposById, draggedId, target, onPointerDown, shouldIgnoreClick }: LaneListProps) {
  const dropIndex = target !== null && target.kind === "list" && target.lane === lane ? target.index : null;
  const isEmpty = tickets.filter((t) => t.id !== draggedId).length === 0;
  let position = 0;

  return (
    <div className={`lane-list${dropIndex !== null ? " lane-list--target" : ""}`} {...{ [DROP_LIST_ATTR]: lane }}>
      {tickets.map((ticket) => {
        const isSource = ticket.id === draggedId;
        const showLine = !isSource && dropIndex === position;
        if (!isSource) {
          position += 1;
        }
        return (
          <Fragment key={ticket.id}>
            {showLine && <div className="drop-line" aria-hidden="true" />}
            <TicketCard
              ticket={ticket}
              stages={stages}
              repo={ticket.repo_id === null ? undefined : reposById.get(ticket.repo_id)}
              isDragSource={isSource}
              onPointerDown={onPointerDown}
              shouldIgnoreClick={shouldIgnoreClick}
            />
          </Fragment>
        );
      })}
      {dropIndex !== null && dropIndex >= position && <div className="drop-line" aria-hidden="true" />}
      {isEmpty && <p className="lane-list__empty">{LANE_EMPTY_HINTS[lane]}</p>}
    </div>
  );
}
