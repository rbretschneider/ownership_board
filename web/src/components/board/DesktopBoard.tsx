import { OwnerBadge } from "../common/OwnerBadge";
import type { BoardViewProps } from "./BoardViewProps";
import { LaneList } from "./LaneList";

/** Six lane columns side by side. */
export function DesktopBoard({ lanes, ticketsByLane, ...listProps }: BoardViewProps) {
  return (
    <div className="columns">
      {lanes.map((lane) => {
        const tickets = ticketsByLane.get(lane.id) ?? [];
        return (
          <section key={lane.id} className={`column column--${lane.owner}`} aria-labelledby={`lane-${lane.id}`}>
            <header className="column__header" title={lane.description}>
              <h2 id={`lane-${lane.id}`} className="column__title">{lane.label}</h2>
              <span className="column__count" aria-label={`${tickets.length} tickets`}>{tickets.length}</span>
              <OwnerBadge owner={lane.owner} />
            </header>
            <LaneList lane={lane.id} tickets={tickets} {...listProps} />
          </section>
        );
      })}
    </div>
  );
}
