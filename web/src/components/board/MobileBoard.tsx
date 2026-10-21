import type { LaneId } from "../../api/types";
import { OwnerBadge } from "../common/OwnerBadge";
import type { BoardViewProps } from "./BoardViewProps";
import { LaneList } from "./LaneList";
import { LaneStrip } from "./LaneStrip";

/** Props for {@link MobileBoard}. */
export interface MobileBoardProps extends BoardViewProps {
  activeLane: LaneId;
  onSelectLane: (lane: LaneId) => void;
}

/** One lane at a time, switched with a sticky pill strip. */
export function MobileBoard({ lanes, ticketsByLane, activeLane, onSelectLane, ...listProps }: MobileBoardProps) {
  const lane = lanes.find((l) => l.id === activeLane) ?? lanes[0];
  const counts = new Map([...ticketsByLane].map(([id, tickets]) => [id, tickets.length]));
  if (lane === undefined) {
    return null;
  }
  return (
    <div className="mobile-board">
      <LaneStrip lanes={lanes} counts={counts} activeLane={lane.id} target={listProps.target} onSelect={onSelectLane} />
      <section className="mobile-lane" aria-labelledby="mobile-lane-title">
        <header className="mobile-lane__header">
          <h2 id="mobile-lane-title" className="mobile-lane__title">{lane.label}</h2>
          <OwnerBadge owner={lane.owner} />
        </header>
        <p className="mobile-lane__description">{lane.description}</p>
        <LaneList lane={lane.id} tickets={ticketsByLane.get(lane.id) ?? []} {...listProps} />
      </section>
    </div>
  );
}
