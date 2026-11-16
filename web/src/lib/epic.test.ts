import { describe, expect, it } from "vitest";
import type { SplitChild } from "../api/types";
import { epicPartState, epicProgress } from "./epic";

const child = (id: number, stage: SplitChild["stage"], lane: SplitChild["lane"]): SplitChild => ({ id, title: `Story ${id}`, stage, lane });

describe("epicProgress", () => {
  it("is null for a ticket that was not split", () => {
    expect(epicProgress({})).toBeNull();
    expect(epicProgress({ split_into: [] })).toBeNull();
  });

  it("counts deployed and merged stories and keeps their order", () => {
    const progress = epicProgress({
      split_into: [child(1, "deployed", "deployed"), child(2, "done", "merged"), child(3, "build", "in_progress"), child(4, "ready", "todo")],
    });
    expect(progress?.total).toBe(4);
    expect(progress?.deployed).toBe(1);
    expect(progress?.merged).toBe(1);
    expect(progress?.label).toBe("1 of 4 deployed");
    expect(progress?.parts.map((p) => p.state)).toEqual(["deployed", "merged", "active", "waiting"]);
  });
});

describe("epicPartState", () => {
  it("treats a story being deployed as merged and a nested epic as active", () => {
    expect(epicPartState({ stage: "deploying", lane: "merged" })).toBe("merged");
    expect(epicPartState({ stage: "epic", lane: "in_progress" })).toBe("active");
    expect(epicPartState({ stage: "spec_review", lane: "todo" })).toBe("waiting");
  });
});
