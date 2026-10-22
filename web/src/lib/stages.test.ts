import { describe, expect, it } from "vitest";
import { ApiError } from "../api/ApiError";
import type { Stage, StageId, TicketDetail, TicketSummary } from "../api/types";
import { COLUMN_GAP_PX, COLUMN_MIN_WIDTH_PX, COLUMNS_PADDING_PX, DESKTOP_MIN_WIDTH_PX } from "./constants";
import {
  checkpointDocument,
  checkpointFor,
  isApprovedRevision,
  isStaleConflict,
  needsOwner,
  requestChangesError,
  stageCounts,
  stageLabel,
  stageSummary,
} from "./stages";

const STAGES: Stage[] = [
  { id: "refine", lane: "todo", label: "Refining", is_owner_review: false },
  { id: "spec_review", lane: "todo", label: "Spec ready", is_owner_review: true },
  { id: "ready", lane: "todo", label: "Ready to build", is_owner_review: false },
  { id: "design", lane: "in_progress", label: "Designing", is_owner_review: false },
  { id: "design_review", lane: "in_progress", label: "Design ready", is_owner_review: true },
  { id: "build", lane: "in_progress", label: "Building", is_owner_review: false },
  { id: "pr", lane: "review", label: "PR open", is_owner_review: true },
  { id: "done", lane: "merged", label: "Merged", is_owner_review: false },
];

function ticket(id: number, stage: StageId, overrides: Partial<TicketSummary> = {}): TicketSummary {
  const lane = STAGES.find((s) => s.id === stage)?.lane ?? "todo";
  return {
    id, lane, stage, repo_id: 1, kind: "story", title: `t${id}`, owner: "agent",
    needs_owner: false, blocked_reason: "", spec_revision: 0, design_revision: 0,
    pr_url: "", branch: "", open_questions: 0, lease: null,
    created_at: "", updated_at: "", lane_changed_at: "", ...overrides,
  };
}

describe("stageCounts", () => {
  const tickets = [
    ticket(1, "refine"), ticket(2, "refine"), ticket(3, "refine"),
    ticket(4, "spec_review"), ticket(5, "ready"), ticket(6, "ready"), ticket(7, "build"),
  ];

  it("counts each stage of one lane in workflow order", () => {
    expect(stageCounts(STAGES, tickets, "todo")).toEqual([
      { id: "refine", label: "Refining", count: 3 },
      { id: "spec_review", label: "Spec ready", count: 1 },
      { id: "ready", label: "Ready to build", count: 2 },
    ]);
  });
  it("leaves out empty stages and other lanes", () => {
    expect(stageCounts(STAGES, tickets, "in_progress")).toEqual([{ id: "build", label: "Building", count: 1 }]);
    expect(stageCounts(STAGES, tickets, "merged")).toEqual([]);
  });
  it("formats the compact summary line", () => {
    expect(stageSummary(stageCounts(STAGES, tickets, "todo"))).toBe("Refining 3 · Spec ready 1 · Ready to build 2");
    expect(stageSummary([])).toBe("");
  });
});

describe("stageLabel", () => {
  it("uses the server label and falls back to the id", () => {
    expect(stageLabel(STAGES, "pr")).toBe("PR open");
    expect(stageLabel([], "pr")).toBe("pr");
  });
});

describe("needsOwner", () => {
  it("filters on needs_owner, not on the lane", () => {
    const tickets = [
      ticket(1, "refine", { needs_owner: true, open_questions: 2 }),
      ticket(2, "build", { needs_owner: true, blocked_reason: "QA failed twice" }),
      ticket(3, "pr", { needs_owner: true }),
      ticket(4, "ready"),
      ticket(5, "done"),
    ];
    expect(needsOwner(tickets).map((t) => t.id)).toEqual([1, 2, 3]);
  });
});

describe("checkpoints", () => {
  it("maps only review stages to a checkpoint", () => {
    expect(checkpointFor("spec_review")).toBe("spec");
    expect(checkpointFor("design_review")).toBe("design");
    for (const stage of ["refine", "ready", "design", "build", "pr", "done"] as const) {
      expect(checkpointFor(stage)).toBeNull();
    }
  });
  it("picks the document and revision the owner is shown", () => {
    const detail = { ...ticket(1, "design_review"), spec: "S", design: "D", spec_revision: 2, design_revision: 5 } as TicketDetail;
    expect(checkpointDocument(detail, "spec")).toEqual({ text: "S", revision: 2 });
    expect(checkpointDocument(detail, "design")).toEqual({ text: "D", revision: 5 });
  });
  it("shows Approved only for the current, non-zero revision", () => {
    expect(isApprovedRevision(3, 3)).toBe(true);
    expect(isApprovedRevision(2, 3)).toBe(false);
    expect(isApprovedRevision(0, 0)).toBe(false);
  });
  it("requires a comment to request changes", () => {
    expect(requestChangesError("")).not.toBeNull();
    expect(requestChangesError("   \n ")).not.toBeNull();
    expect(requestChangesError("Split the migration")).toBeNull();
  });
  it("treats only a 409 as a stale checkpoint", () => {
    expect(isStaleConflict(new ApiError(409, "conflict", "changed"))).toBe(true);
    expect(isStaleConflict(new ApiError(400, "invalid", "bad"))).toBe(false);
    expect(isStaleConflict(new Error("409"))).toBe(false);
  });
});

describe("desktop layout", () => {
  it("fits four columns side by side on a vertical 1080p monitor", () => {
    const needed = 4 * COLUMN_MIN_WIDTH_PX + 3 * COLUMN_GAP_PX + 2 * COLUMNS_PADDING_PX;
    expect(needed).toBeLessThanOrEqual(DESKTOP_MIN_WIDTH_PX);
    expect(DESKTOP_MIN_WIDTH_PX).toBeLessThanOrEqual(1080);
  });
});
