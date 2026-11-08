import type { Distribution, NamedCount, StageId, StatsReport, StatsSegment } from "../api/types";

/** The ticket groups the stats page compares. */
export type SegmentKey = "autonomous" | "owner";

/** Display names of the segments, in the order they are shown. */
export const SEGMENTS: { key: SegmentKey; label: string }[] = [
  { key: "autonomous", label: "Autonomous" },
  { key: "owner", label: "Yours" },
];

/** Periods the stats page offers, in days. */
export const STATS_PERIODS = [7, 30, 90] as const;

/** The default stats period, in days. */
export const DEFAULT_STATS_PERIOD = 30;

/** The period the board's summary strip reports, in days. */
export const SUMMARY_PERIOD = 7;

/** How often the summary strip refreshes. */
export const SUMMARY_REFRESH_MS = 5 * 60_000;

const HOURS_PER_DAY = 24;
const MINUTES_PER_HOUR = 60;
/** Durations under this many days read in hours. */
const HOURS_THRESHOLD_DAYS = 2;

/** Short, workflow-shaped names for stages; the board's labels are for cards. */
const STAGE_NAMES: Record<StageId, string> = {
  refine: "Refine",
  spec_review: "Spec review",
  ready: "Ready",
  design: "Design",
  design_review: "Design review",
  build: "Build",
  pr: "Review (PR)",
  done: "Merged",
  deploying: "Deploying",
  deployed: "Deployed",
};

/** The stage's short workflow name. */
export function stageName(stage: StageId): string {
  return STAGE_NAMES[stage] ?? stage;
}

/** Formats hours as "45 min", "5.5 h" or "3.2 d". */
export function formatHours(hours: number): string {
  if (hours < 1) {
    return `${Math.round(hours * MINUTES_PER_HOUR)} min`;
  }
  if (hours < HOURS_THRESHOLD_DAYS * HOURS_PER_DAY) {
    return `${roundTenth(hours)} h`;
  }
  return `${roundTenth(hours / HOURS_PER_DAY)} d`;
}

/** The median of a distribution, or an em dash when there is no data. */
export function formatMedian(distribution: Distribution): string {
  return distribution.count === 0 ? "—" : formatHours(distribution.median_hours);
}

/** The 90th percentile of a distribution, or an em dash when there is no data. */
export function formatP90(distribution: Distribution): string {
  return distribution.count === 0 ? "—" : formatHours(distribution.p90_hours);
}

/** One day of the throughput chart, with merges per segment. */
export interface ThroughputDay {
  date: string;
  autonomous: number;
  owner: number;
}

/** Merges per day for both segments; the backend lists the same days for each. */
export function throughputDays(report: StatsReport): ThroughputDay[] {
  return report.autonomous.daily.map((day, i) => ({
    date: day.date,
    autonomous: day.merged,
    owner: report.owner.daily[i]?.merged ?? 0,
  }));
}

/** The busiest day's total merges, at least 1 so a chart scale never divides by zero. */
export function busiestDay(days: ThroughputDay[]): number {
  return Math.max(1, ...days.map((d) => d.autonomous + d.owner));
}

/** Open tickets across every stage of a segment. */
export function openCount(segment: StatsSegment): number {
  return segment.wip.reduce((sum, stage) => sum + stage.count, 0);
}

/** One name's tally in both segments. */
export interface PairedCount {
  name: string;
  autonomous: number;
  owner: number;
}

/** Joins two segments' tallies by name, largest combined total first. */
export function pairCounts(autonomous: NamedCount[], owner: NamedCount[]): PairedCount[] {
  const byName = new Map<string, PairedCount>();
  const row = (name: string) => byName.get(name) ?? { name, autonomous: 0, owner: 0 };
  for (const item of autonomous) {
    byName.set(item.name, { ...row(item.name), autonomous: item.count });
  }
  for (const item of owner) {
    byName.set(item.name, { ...row(item.name), owner: item.count });
  }
  return [...byName.values()].sort((a, b) => b.autonomous + b.owner - (a.autonomous + a.owner) || a.name.localeCompare(b.name));
}

/** A local YYYY-MM-DD day as "Mar 4". */
export function shortDay(date: string): string {
  const [year, month, day] = date.split("-").map(Number);
  if (year === undefined || month === undefined || day === undefined) {
    return date;
  }
  return new Date(year, month - 1, day).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

/** Reads a whole number of days from the stats route, falling back to the default. */
export function periodFromParam(raw: string | null): number {
  const days = Number(raw);
  return STATS_PERIODS.some((p) => p === days) ? days : DEFAULT_STATS_PERIOD;
}

function roundTenth(value: number): number {
  return Math.round(value * 10) / 10;
}
