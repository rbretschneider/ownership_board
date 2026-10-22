import type { LaneId } from "../../api/types";
import { needsOwner } from "../../lib/stages";
import type { BoardViewProps } from "./BoardViewProps";
import { LaneList } from "./LaneList";
import { LaneStrip } from "./LaneStrip";
import { StageSummary } from "./StageSummary";

/** Props for {@link MobileBoard}. */
export interface MobileBoardProps extends BoardViewProps {
  activeLane: LaneId;
  onSelectLane: (lane: LaneId) => void;
}

/** One lane at a time, switched with a sticky pill strip. */
export function MobileBoard({ lanes, stages, ticketsByLane, activeLane, onSelectLane, ...listProps }: MobileBoardProps) {
  const lane = lanes.find((l) => l.id === activeLane) ?? lanes[0];
  const counts = new Map([...ticketsByLane].map(([id, tickets]) => [id, tickets.length]));
  const waiting = new Map([...ticketsByLane].map(([id, tickets]) => [id, needsOwner(tickets).length]));
  if (lane === undefined) {
    return null;
  }
  const tickets = ticketsByLane.get(lane.id) ?? [];
  return (
    <div className="mobile-board">
      <LaneStrip lanes={lanes} counts={counts} waiting={waiting} activeLane={lane.id} target={listProps.target} onSelect={onSelectLane} />
      <section className="mobile-lane" aria-labelledby="mobile-lane-title">
        <header className="mobile-lane__header">
          <h2 id="mobile-lane-title" className="mobile-lane__title">{lane.label}</h2>
        </header>
        <p className="mobile-lane__description">{lane.description}</p>
        <StageSummary stages={stages} tickets={tickets} lane={lane.id} />
        <LaneList lane={lane.id} tickets={tickets} stages={stages} {...listProps} />
      </section>
    </div>
  );
}
