import { describe, expect, it } from "vitest";
import type { StatsReport } from "../api/types";
import { statsSegment as segment } from "../components/stats/statsFixture";
import { busiestDay, formatHours, formatMedian, openCount, pairCounts, periodFromParam, throughputDays } from "./stats";

describe("formatHours", () => {
  it("uses minutes under an hour, hours under two days, then days", () => {
    expect(formatHours(0.25)).toBe("15 min");
    expect(formatHours(5.55)).toBe("5.6 h");
    expect(formatHours(60)).toBe("2.5 d");
  });
});

describe("formatMedian", () => {
  it("shows no data as a dash rather than zero", () => {
    expect(formatMedian({ count: 0, median_hours: 0, p90_hours: 0 })).toBe("—");
    expect(formatMedian({ count: 3, median_hours: 2, p90_hours: 4 })).toBe("2 h");
  });
});

describe("throughputDays", () => {
  it("pairs each day's autonomous and owner merges", () => {
    const report = {
      autonomous: segment({ daily: [{ date: "2026-03-09", merged: 2 }, { date: "2026-03-10", merged: 0 }] }),
      owner: segment({ daily: [{ date: "2026-03-09", merged: 1 }, { date: "2026-03-10", merged: 3 }] }),
    } as StatsReport;
    const days = throughputDays(report);
    expect(days).toEqual([
      { date: "2026-03-09", autonomous: 2, owner: 1 },
      { date: "2026-03-10", autonomous: 0, owner: 3 },
    ]);
    expect(busiestDay(days)).toBe(3);
  });
  it("keeps the chart scale positive when nothing merged", () => {
    expect(busiestDay([{ date: "2026-03-10", autonomous: 0, owner: 0 }])).toBe(1);
  });
});

describe("openCount", () => {
  it("sums open tickets across stages", () => {
    const wip = [{ stage: "refine", lane: "todo", count: 2 }, { stage: "build", lane: "in_progress", count: 1 }] as const;
    expect(openCount(segment({ wip: [...wip] }))).toBe(3);
  });
});

describe("pairCounts", () => {
  it("joins both segments by name, busiest first", () => {
    const rows = pairCounts([{ name: "design", count: 1 }, { name: "implement", count: 4 }], [{ name: "design", count: 2 }]);
    expect(rows).toEqual([
      { name: "implement", autonomous: 4, owner: 0 },
      { name: "design", autonomous: 1, owner: 2 },
    ]);
  });
});

describe("periodFromParam", () => {
  it("accepts only the offered periods", () => {
    expect(periodFromParam("7")).toBe(7);
    expect(periodFromParam("12")).toBe(30);
    expect(periodFromParam(null)).toBe(30);
  });
});
