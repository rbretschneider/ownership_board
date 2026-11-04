import type { StatsReport, StatsSegment } from "../../api/types";

const NO_DATA = { count: 0, median_hours: 0, p90_hours: 0 };

/** A segment with nothing recorded, overridable per test. */
export function statsSegment(overrides: Partial<StatsSegment> = {}): StatsSegment {
  return {
    merged_24h: 0, merged: 0, prs_opened: 0, created: 0, daily: [], lead_time: NO_DATA, cycle_time: NO_DATA,
    stage_times: [], wip: [], blocked: 0, needs_owner: 0, repair_rounds: 0, repaired_tickets: 0,
    changes_requested: 0, models: [], claims: [], agents: [], ...overrides,
  };
}

/** A report from autonomous and owner segments; `all` sums the merge counts. */
export function statsReport(autonomous: StatsSegment, owner: StatsSegment, days = 7): StatsReport {
  return {
    generated_at: "2026-03-10T15:00:00Z",
    window_days: days,
    since: "2026-03-04T00:00:00Z",
    all: statsSegment({
      merged_24h: autonomous.merged_24h + owner.merged_24h,
      merged: autonomous.merged + owner.merged,
      cycle_time: autonomous.cycle_time,
    }),
    autonomous,
    owner,
    repos: [],
  };
}
