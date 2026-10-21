import type { Lane, LaneId } from "../../api/types";
import { LANE_SHORT_LABELS, OWNER_LABELS } from "../../lib/constants";
import { DROP_PILL_ATTR, type DropTarget } from "../../lib/dropTargets";

/** Props for {@link LaneStrip}. */
export interface LaneStripProps {
  lanes: Lane[];
  counts: Map<LaneId, number>;
  activeLane: LaneId;
  target: DropTarget | null;
  onSelect: (lane: LaneId) => void;
}

/** Mobile lane switcher. While dragging, each pill is also a drop target. */
export function LaneStrip({ lanes, counts, activeLane, target, onSelect }: LaneStripProps) {
  return (
    <nav className="lane-strip" aria-label="Lanes">
      {lanes.map((lane) => {
        const count = counts.get(lane.id) ?? 0;
        const isTarget = target?.kind === "pill" && target.lane === lane.id;
        const isActive = lane.id === activeLane;
        return (
          <button
            key={lane.id}
            type="button"
            className={`pill pill--${lane.owner}${isActive ? " pill--active" : ""}${isTarget ? " pill--target" : ""}`}
            aria-pressed={isActive}
            aria-label={`${lane.label}, ${count} tickets, owner ${OWNER_LABELS[lane.owner]}`}
            onClick={() => onSelect(lane.id)}
            {...{ [DROP_PILL_ATTR]: lane.id }}
          >
            {lane.owner === "human" && count > 0 && <span className="pill__dot" aria-hidden="true" />}
            <span>{LANE_SHORT_LABELS[lane.id]}</span>
            <span className="pill__count">{count}</span>
          </button>
        );
      })}
    </nav>
  );
}
