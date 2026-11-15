import { describe, expect, it } from "vitest";
import { buildClockText, formatElapsed } from "./buildClock";

describe("formatElapsed", () => {
  it("uses seconds, then minutes, then hours and minutes", () => {
    expect(formatElapsed(45_000)).toBe("45s");
    expect(formatElapsed(12 * 60_000 + 30_000)).toBe("12m");
    expect(formatElapsed(65 * 60_000)).toBe("1h 05m");
    expect(formatElapsed(-5)).toBe("0s");
  });
});

describe("buildClockText", () => {
  const now = Date.parse("2026-11-15T12:00:00Z");
  it("counts a running job from its start and says how long it waited", () => {
    const clock = buildClockText({ status: "running", queued_at: "2026-11-15T10:00:00Z", started_at: "2026-11-15T10:30:00Z" }, now);
    expect(clock).toEqual({ text: "1h 30m", title: "Build running for 1h 30m after waiting 30m" });
  });
  it("counts a queued job from when it was queued", () => {
    expect(buildClockText({ status: "queued", queued_at: "2026-11-15T11:48:00Z", started_at: null }, now))
      .toEqual({ text: "12m", title: "Build queued for 12m" });
  });
  it("shows nothing without a job", () => {
    expect(buildClockText(undefined, now)).toBeUndefined();
    expect(buildClockText(null, now)).toBeUndefined();
  });
});
