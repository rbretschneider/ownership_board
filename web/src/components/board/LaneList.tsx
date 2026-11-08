import { BookOpen, Bug } from "lucide-react";
import { Fragment, useState, type PointerEvent } from "react";
import type { Kind, LaneId, Repo, Stage, TicketSummary } from "../../api/types";
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
  /** Opens the create dialog for that kind; offered in an empty Todo lane. */
  onQuickAdd?: (kind: Kind) => void;
}

/** The cards of one lane, plus the drop indicator while dragging. */
export function LaneList({
  lane,
  tickets,
  stages,
  reposById,
  draggedId,
  target,
  onPointerDown,
  shouldIgnoreClick,
  onQuickAdd,
}: LaneListProps) {
  const [showAllMerged, setShowAllMerged] = useState(false);
  const hasMore = lane === "merged" && tickets.length > 5;
  const shown = hasMore && !showAllMerged ? tickets.slice(0, 5) : tickets;
  const dropIndex = target !== null && target.kind === "list" && target.lane === lane ? target.index : null;
  const isEmpty = tickets.filter((t) => t.id !== draggedId).length === 0;
  let position = 0;

  return (
    <div className={`lane-list${dropIndex !== null ? " lane-list--target" : ""}`} {...{ [DROP_LIST_ATTR]: lane }}>
      {shown.map((ticket) => {
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
      {hasMore && (
        <button type="button" className="button button--secondary lane-list__more" aria-expanded={showAllMerged} onClick={() => setShowAllMerged((value) => !value)}>
          {showAllMerged ? "Show fewer" : `See ${tickets.length - 5} more`}
        </button>
      )}
      {isEmpty && <p className="lane-list__empty">{LANE_EMPTY_HINTS[lane]}</p>}
      {isEmpty && lane === "todo" && onQuickAdd !== undefined && (
        <div className="lane-list__quick-add">
          <button type="button" className="button button--secondary" onClick={() => onQuickAdd("story")}>
            <BookOpen size={16} aria-hidden="true" />
            New story
          </button>
          <button type="button" className="button button--secondary" onClick={() => onQuickAdd("bug")}>
            <Bug size={16} aria-hidden="true" />
            New bug
          </button>
        </div>
      )}
    </div>
  );
}
