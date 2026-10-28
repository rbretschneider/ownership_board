import { describe, expect, it } from "vitest";
import type { LaneId, TicketSummary } from "../api/types";
import { afterIdFor, applyMove, groupByLane, insertIndexFromMidpoints } from "./position";

function ticket(id: number, lane: LaneId): TicketSummary {
  return {
    id, lane, repo_id: 1, kind: "story", title: `t${id}`, owner: "agent",
    stage: "refine", needs_owner: false, blocked_reason: "", is_autonomous: false, spec_revision: 0, design_revision: 0,
    pr_url: "", branch: "", open_questions: 0, lease: null,
    created_at: "", updated_at: "", lane_changed_at: "",
  };
}

const order = (tickets: TicketSummary[]) => tickets.map((t) => `${t.lane}:${t.id}`);

describe("insertIndexFromMidpoints", () => {
  const mids = [100, 200, 300];
  it("inserts at the top above the first midpoint", () => {
    expect(insertIndexFromMidpoints(mids, 50)).toBe(0);
  });
  it("inserts between cards", () => {
    expect(insertIndexFromMidpoints(mids, 250)).toBe(2);
  });
  it("inserts at the end below the last midpoint", () => {
    expect(insertIndexFromMidpoints(mids, 999)).toBe(3);
  });
  it("inserts at the top of an empty list", () => {
    expect(insertIndexFromMidpoints([], 10)).toBe(0);
  });
});

describe("afterIdFor", () => {
  it("returns null for the top", () => {
    expect(afterIdFor([4, 5, 6], 9, 0)).toBeNull();
  });
  it("returns the card above the drop position", () => {
    expect(afterIdFor([4, 5, 6], 9, 2)).toBe(5);
  });
  it("ignores the dragged card when reordering in place", () => {
    expect(afterIdFor([4, 5, 6], 4, 2)).toBe(6);
  });
  it("clamps past-the-end positions to the last card", () => {
    expect(afterIdFor([4, 5], 9, 7)).toBe(5);
  });
  it("returns null for an empty lane", () => {
    expect(afterIdFor([], 9, 3)).toBeNull();
  });
});

describe("applyMove", () => {
  const board = [ticket(1, "todo"), ticket(2, "todo"), ticket(3, "review"), ticket(4, "review")];

  it("moves a card to the top of another lane", () => {
    expect(order(applyMove(board, 1, "review", null))).toEqual(["todo:2", "review:1", "review:3", "review:4"]);
  });
  it("places a card after the given card", () => {
    expect(order(applyMove(board, 1, "review", 3))).toEqual(["todo:2", "review:3", "review:1", "review:4"]);
  });
  it("reorders within a lane", () => {
    expect(order(applyMove(board, 1, "todo", 2))).toEqual(["todo:2", "todo:1", "review:3", "review:4"]);
  });
  it("appends into an empty lane", () => {
    expect(order(applyMove(board, 3, "merged", null))).toEqual(["todo:1", "todo:2", "review:4", "merged:3"]);
  });
  it("does not modify its input", () => {
    applyMove(board, 1, "merged", null);
    expect(order(board)).toEqual(["todo:1", "todo:2", "review:3", "review:4"]);
  });
  it("returns a copy unchanged for an unknown card", () => {
    expect(order(applyMove(board, 99, "merged", null))).toEqual(order(board));
  });
});

describe("groupByLane", () => {
  it("keeps order and includes empty lanes", () => {
    const groups = groupByLane([ticket(2, "review"), ticket(1, "review")], ["todo", "review"]);
    expect(groups.get("todo")).toEqual([]);
    expect(groups.get("review")?.map((t) => t.id)).toEqual([2, 1]);
  });
});
