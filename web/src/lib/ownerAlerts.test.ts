import { describe, expect, it } from "vitest";
import type { Stage, TicketSummary } from "../api/types";
import { alertText, newlyWaiting, titleFor, waitingIds } from "./ownerAlerts";

function ticket(id: number, overrides: Partial<TicketSummary> = {}): TicketSummary {
  return {
    id, repo_id: null, kind: "story", title: "Private title", lane: "todo", stage: "refine",
    owner: "agent", needs_owner: false, blocked_reason: "", is_autonomous: false, spec_revision: 0, design_revision: 0,
    pr_url: "", branch: "", open_questions: 0, waiting_on: [], lease: null,
    created_at: "", updated_at: "", lane_changed_at: "", ...overrides,
  };
}

const stages: Stage[] = [{ id: "spec_review", lane: "todo", label: "Spec ready", is_owner_review: true }];

describe("titleFor", () => {
  it("shows the waiting count only when something waits", () => {
    expect(titleFor(0)).toBe("ownership");
    expect(titleFor(3)).toBe("(3) ownership");
  });
});

describe("newlyWaiting", () => {
  it("never alerts on the first load", () => {
    expect(newlyWaiting(null, [ticket(1, { needs_owner: true })])).toEqual([]);
  });

  it("alerts only for tickets that started waiting", () => {
    const before = waitingIds([ticket(1, { needs_owner: true }), ticket(2)]);
    const now = [ticket(1, { needs_owner: true }), ticket(2, { needs_owner: true }), ticket(3)];
    expect(newlyWaiting(before, now).map((t) => t.id)).toEqual([2]);
  });
});

describe("alertText", () => {
  it("uses fixed board words, never the ticket title", () => {
    const text = alertText(ticket(7, { needs_owner: true, stage: "spec_review" }), stages);
    expect(text).toEqual({ title: "#7 needs you", body: "Spec ready" });
    expect(JSON.stringify(text)).not.toContain("Private title");
  });

  it("names questions and blocks first", () => {
    expect(alertText(ticket(1, { open_questions: 2 }), stages).body).toBe("2 questions");
    expect(alertText(ticket(1, { blocked_reason: "agent words" }), stages).body).toBe("Blocked");
  });
});
