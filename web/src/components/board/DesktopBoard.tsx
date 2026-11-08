import type { BoardViewProps } from "./BoardViewProps";
import { LaneList } from "./LaneList";
import { StageSummary } from "./StageSummary";

/** Four lane columns side by side. */
export function DesktopBoard({ lanes, stages, ticketsByLane, ...listProps }: BoardViewProps) {
  return (
    <div className="columns">
      {lanes.map((lane) => {
        const tickets = ticketsByLane.get(lane.id) ?? [];
        return (
          <section key={lane.id} className="column" aria-labelledby={`lane-${lane.id}`}>
            <header className="column__header" title={lane.description}>
              <div className="column__heading">
                <h2 id={`lane-${lane.id}`} className="column__title">{lane.label}</h2>
                <span className="column__count" aria-label={`${tickets.length} tickets`}>{tickets.length}</span>
              </div>
              <StageSummary stages={stages} tickets={tickets} lane={lane.id} />
            </header>
            <LaneList lane={lane.id} tickets={tickets} stages={stages} {...listProps} />
          </section>
        );
      })}
    </div>
  );
}
