import type { Lane, LaneId } from "../../api/types";
import { LANE_SHORT_LABELS } from "../../lib/constants";
import { DROP_PILL_ATTR, type DropTarget } from "../../lib/dropTargets";

/** Props for {@link LaneStrip}. */
export interface LaneStripProps {
  lanes: Lane[];
  counts: Map<LaneId, number>;
  /** Tickets per lane that need the owner. */
  waiting: Map<LaneId, number>;
  activeLane: LaneId;
  target: DropTarget | null;
  onSelect: (lane: LaneId) => void;
}

/** Mobile lane switcher. While dragging, each pill is also a drop target. */
export function LaneStrip({ lanes, counts, waiting, activeLane, target, onSelect }: LaneStripProps) {
  return (
    <nav className="lane-strip" aria-label="Lanes">
      {lanes.map((lane) => {
        const count = counts.get(lane.id) ?? 0;
        const waitingCount = waiting.get(lane.id) ?? 0;
        const isTarget = target?.kind === "pill" && target.lane === lane.id;
        const isActive = lane.id === activeLane;
        const waitingText = waitingCount > 0 ? `, ${waitingCount} need you` : "";
        return (
          <button
            key={lane.id}
            type="button"
            className={`pill${isActive ? " pill--active" : ""}${isTarget ? " pill--target" : ""}`}
            aria-pressed={isActive}
            aria-label={`${lane.label}, ${count} tickets${waitingText}`}
            onClick={() => onSelect(lane.id)}
            {...{ [DROP_PILL_ATTR]: lane.id }}
          >
            {waitingCount > 0 && <span className="pill__dot" aria-hidden="true" />}
            <span>{LANE_SHORT_LABELS[lane.id]}</span>
            <span className="pill__count">{count}</span>
          </button>
        );
      })}
    </nav>
  );
}
