import { describe, expect, it } from "vitest";
import { relativeTime } from "./relativeTime";

const now = new Date(2026, 9, 21, 12, 0, 0);
const ago = (seconds: number) => new Date(now.getTime() - seconds * 1000).toISOString();

describe("relativeTime", () => {
  it("says just now for recent times", () => {
    expect(relativeTime(ago(10), now)).toBe("just now");
  });
  it("treats future times as just now", () => {
    expect(relativeTime(ago(-120), now)).toBe("just now");
  });
  it("uses minutes under an hour", () => {
    expect(relativeTime(ago(5 * 60), now)).toBe("5m ago");
  });
  it("uses hours under a day", () => {
    expect(relativeTime(ago(3 * 3600 + 100), now)).toBe("3h ago");
  });
  it("uses days under a week", () => {
    expect(relativeTime(ago(2 * 86400), now)).toBe("2d ago");
  });
  it("uses a short date in the same year", () => {
    expect(relativeTime(new Date(2026, 2, 4).toISOString(), now)).toBe("Mar 4");
  });
  it("adds the year for other years", () => {
    expect(relativeTime(new Date(2025, 2, 4).toISOString(), now)).toBe("Mar 4, 2025");
  });
  it("returns empty text for invalid input", () => {
    expect(relativeTime("nope", now)).toBe("");
  });
});
