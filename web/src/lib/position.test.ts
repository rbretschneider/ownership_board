import { describe, expect, it } from "vitest";
import type { LaneId, TicketSummary } from "../api/types";
import { afterIdFor, applyMove, groupByLane, insertIndexFromMidpoints } from "./position";

function ticket(id: number, lane: LaneId): TicketSummary {
  return {
    id, lane, repo_id: 1, kind: "story", title: `t${id}`, owner: "agent",
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
  const board = [ticket(1, "new"), ticket(2, "new"), ticket(3, "ready"), ticket(4, "ready")];

  it("moves a card to the top of another lane", () => {
    expect(order(applyMove(board, 1, "ready", null))).toEqual(["new:2", "ready:1", "ready:3", "ready:4"]);
  });
  it("places a card after the given card", () => {
    expect(order(applyMove(board, 1, "ready", 3))).toEqual(["new:2", "ready:3", "ready:1", "ready:4"]);
  });
  it("reorders within a lane", () => {
    expect(order(applyMove(board, 1, "new", 2))).toEqual(["new:2", "new:1", "ready:3", "ready:4"]);
  });
  it("appends into an empty lane", () => {
    expect(order(applyMove(board, 3, "done", null))).toEqual(["new:1", "new:2", "ready:4", "done:3"]);
  });
  it("does not modify its input", () => {
    applyMove(board, 1, "done", null);
    expect(order(board)).toEqual(["new:1", "new:2", "ready:3", "ready:4"]);
  });
  it("returns a copy unchanged for an unknown card", () => {
    expect(order(applyMove(board, 99, "done", null))).toEqual(order(board));
  });
});

describe("groupByLane", () => {
  it("keeps order and includes empty lanes", () => {
    const groups = groupByLane([ticket(2, "ready"), ticket(1, "ready")], ["new", "ready"]);
    expect(groups.get("new")).toEqual([]);
    expect(groups.get("ready")?.map((t) => t.id)).toEqual([2, 1]);
  });
});
