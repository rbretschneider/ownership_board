import type { LaneId, Stage, TicketSummary } from "../../api/types";
import { stageCounts, stageSummary } from "../../lib/stages";

/** The compact per-stage counts under a lane header, e.g. "Refining 3 · Spec ready 1". */
export function StageSummary({ stages, tickets, lane }: { stages: Stage[]; tickets: TicketSummary[]; lane: LaneId }) {
  const counts = stageCounts(stages, tickets, lane);
  if (counts.length === 0) {
    return null;
  }
  return <p className="stage-summary">{stageSummary(counts)}</p>;
}
