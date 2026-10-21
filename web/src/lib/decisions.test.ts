import { describe, expect, it } from "vitest";
import type { Decision } from "../api/types";
import { cleanAlternatives, decisionRound, deleteMessage, groupDecisions, replacementSummary } from "./decisions";

function decision(id: number, overrides: Partial<Decision> = {}): Decision {
  return {
    id,
    summary: `decision ${id}`,
    rationale: "",
    alternatives: [],
    source: "agent",
    weight: "agent",
    decided_by: { kind: "agent", name: "loop-a" },
    asked_by: "",
    question_id: null,
    supersedes_id: null,
    superseded_by_id: null,
    is_removed: false,
    at: `2026-01-0${id}T00:00:00Z`,
    ...overrides,
  };
}

describe("groupDecisions", () => {
  it("lists binding, then delegated, then agent decisions, newest first within each", () => {
    const groups = groupDecisions([
      decision(1, { weight: "agent" }),
      decision(2, { weight: "binding" }),
      decision(3, { weight: "delegated" }),
      decision(4, { weight: "binding" }),
      decision(5, { weight: "agent" }),
    ]);
    expect(groups.active.map((d) => d.id)).toEqual([4, 2, 3, 5, 1]);
  });
  it("moves superseded and removed decisions to the history", () => {
    const groups = groupDecisions([
      decision(1, { superseded_by_id: 3 }),
      decision(2, { is_removed: true, summary: "" }),
      decision(3, { supersedes_id: 1 }),
    ]);
    expect(groups.active.map((d) => d.id)).toEqual([3]);
    expect(groups.history.map((d) => d.id)).toEqual([2, 1]);
  });
});

describe("replacementSummary", () => {
  it("names the decision that replaced this one", () => {
    const all = [decision(1, { superseded_by_id: 2 }), decision(2, { summary: "use SQLite" })];
    expect(replacementSummary(all[0]!, all)).toBe("use SQLite");
  });
  it("is null for an active decision or a removed replacement", () => {
    const all = [decision(1, { superseded_by_id: 2 }), decision(2, { is_removed: true })];
    expect(replacementSummary(all[1]!, all)).toBeNull();
    expect(replacementSummary(all[0]!, all)).toBeNull();
  });
});

describe("decisionRound", () => {
  it("finds the round of the question an answer-derived decision came from", () => {
    const questions = [{ id: 9, round: 2 }] as unknown as Parameters<typeof decisionRound>[1];
    expect(decisionRound(decision(1, { question_id: 9 }), questions)).toBe(2);
    expect(decisionRound(decision(1), questions)).toBeNull();
  });
});

describe("deleteMessage", () => {
  it("says the replaced decision comes back when deleting a replacement", () => {
    expect(deleteMessage(decision(2, { supersedes_id: 1 }))).toContain("becomes active again");
    expect(deleteMessage(decision(2))).not.toContain("becomes active again");
  });
});

describe("cleanAlternatives", () => {
  it("drops blanks and keeps at most five", () => {
    expect(cleanAlternatives([" a ", "", "b", "c", "d", "e", "f"])).toEqual(["a", "b", "c", "d", "e"]);
  });
});
