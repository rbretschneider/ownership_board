import type { StatsReport, StatsSegment } from "../../api/types";
import { formatMedian, formatP90, openCount, pairCounts, stageName } from "../../lib/stats";
import type { CompareRow } from "./CompareTable";

/** Builds a row from one value per segment. */
function row(label: string, value: (s: StatsSegment) => string | number, hint?: string): (r: StatsReport) => CompareRow {
  return (report) => ({ label, hint, values: { autonomous: value(report.autonomous), owner: value(report.owner) } });
}

const DELIVERY_ROWS = [
  row("Merged, last 24 h", (s) => s.merged_24h),
  row("Merged", (s) => s.merged),
  row("PRs opened", (s) => s.prs_opened, "First time a ticket reached review"),
  row("Created", (s) => s.created),
  row("Lead time, median", (s) => formatMedian(s.lead_time), "Created to merged"),
  row("Lead time, p90", (s) => formatP90(s.lead_time)),
  row("Cycle time, median", (s) => formatMedian(s.cycle_time), "First in progress to merged"),
  row("Cycle time, p90", (s) => formatP90(s.cycle_time)),
];

const QUALITY_ROWS = [
  row("Repair rounds", (s) => s.repair_rounds, "Review sent back to build: failed check, conflict or requested change"),
  row("Tickets repaired", (s) => s.repaired_tickets),
  row("Checkpoint changes requested", (s) => s.changes_requested, "You asked for changes to a spec or design"),
];

const NOW_ROWS = [
  row("Open", openCount),
  row("Waiting on you", (s) => s.needs_owner, "Questions, blocks and checkpoints"),
  row("Blocked", (s) => s.blocked),
];

/** Throughput and timing for the period. */
export function deliveryRows(report: StatsReport): CompareRow[] {
  return DELIVERY_ROWS.map((build) => build(report));
}

/** How often work bounced back. */
export function qualityRows(report: StatsReport): CompareRow[] {
  return QUALITY_ROWS.map((build) => build(report));
}

/** Open work right now, overall. */
export function nowRows(report: StatsReport): CompareRow[] {
  return NOW_ROWS.map((build) => build(report));
}

/** Open tickets per stage right now. */
export function wipRows(report: StatsReport): CompareRow[] {
  return report.autonomous.wip.map((stage, i) => ({
    label: stageName(stage.stage),
    values: { autonomous: stage.count, owner: report.owner.wip[i]?.count ?? 0 },
  }));
}

/** Median and p90 time in each stage, for stays that ended in the period. */
export function stageTimeRows(report: StatsReport): CompareRow[] {
  return report.autonomous.stage_times.map((stage, i) => {
    const owner = report.owner.stage_times[i] ?? { count: 0, median_hours: 0, p90_hours: 0 };
    const cell = (d: typeof owner) => (d.count === 0 ? "—" : `${formatMedian(d)} · p90 ${formatP90(d)} (${d.count})`);
    return { label: stageName(stage.stage), values: { autonomous: cell(stage), owner: cell(owner) } };
  });
}

/** A tally such as models or actions, joined across both segments. */
export function mixRows(report: StatsReport, pick: (s: StatsSegment) => StatsSegment["models"]): CompareRow[] {
  return pairCounts(pick(report.autonomous), pick(report.owner)).map((pair) => ({
    label: pair.name,
    values: { autonomous: pair.autonomous, owner: pair.owner },
  }));
}
