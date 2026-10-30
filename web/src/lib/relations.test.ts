import { describe, expect, it } from "vitest";
import type { Relation, TicketSummary } from "../api/types";
import {
  dependenciesMetText,
  dependencyCandidates,
  groupRelations,
  relationEnds,
  relationLabel,
  searchTickets,
  ticketRefs,
} from "./relations";

function relation(kind: Relation["kind"], direction: Relation["direction"], id: number): Relation {
  return {
    kind,
    direction,
    label: "",
    ticket: { id, title: `ticket ${id}`, lane: "todo", visibility: "private" },
    added_by: { kind: "human", name: "Owner" },
    added_at: "2026-01-01T00:00:00Z",
  };
}

describe("relationLabel", () => {
  it("labels each kind from this ticket's side", () => {
    expect(relationLabel(relation("caused_by", "outgoing", 1))).toBe("Caused by");
    expect(relationLabel(relation("caused_by", "incoming", 1))).toBe("Caused");
    expect(relationLabel(relation("follow_up_of", "incoming", 1))).toBe("Has follow-up");
    expect(relationLabel(relation("duplicate_of", "incoming", 1))).toBe("Duplicated by");
    expect(relationLabel(relation("relates_to", "incoming", 1))).toBe("Relates to");
    expect(relationLabel(relation("depends_on", "outgoing", 1))).toBe("Depends on");
    expect(relationLabel(relation("depends_on", "incoming", 1))).toBe("Needed by");
  });
});

describe("groupRelations", () => {
  it("groups in a fixed order and merges both directions of relates-to", () => {
    const groups = groupRelations([
      relation("relates_to", "incoming", 4),
      relation("follow_up_of", "outgoing", 2),
      relation("caused_by", "outgoing", 1),
      relation("relates_to", "outgoing", 5),
    ]);
    expect(groups.map((g) => g.label)).toEqual(["Caused by", "Follow-up of", "Relates to"]);
    expect(groups[2]?.relations.map((r) => r.ticket.id)).toEqual([4, 5]);
  });
});

describe("relationEnds", () => {
  it("deletes an incoming relation from the other ticket's side", () => {
    expect(relationEnds(7, relation("caused_by", "outgoing", 3))).toEqual({ from: 7, to: 3 });
    expect(relationEnds(7, relation("caused_by", "incoming", 3))).toEqual({ from: 3, to: 7 });
  });
});

describe("searchTickets", () => {
  const tickets = [
    { id: 12, title: "Login fails" },
    { id: 120, title: "Dark mode" },
    { id: 3, title: "Login page copy" },
  ] as TicketSummary[];
  it("matches #id prefixes and titles, never the current ticket", () => {
    expect(searchTickets(tickets, "#12", 3).map((t) => t.id)).toEqual([12, 120]);
    expect(searchTickets(tickets, "login", 12).map((t) => t.id)).toEqual([3]);
  });
  it("offers the newest tickets when the query is empty", () => {
    expect(searchTickets(tickets, "", 120).map((t) => t.id)).toEqual([12, 3]);
  });
});

describe("dependencies", () => {
  const tickets = [
    { id: 1, repo_id: 7 },
    { id: 2, repo_id: 8 },
    { id: 3, repo_id: null },
    { id: 4, repo_id: 7 },
  ] as TicketSummary[];
  it("offers only tickets in the same repo", () => {
    expect(dependencyCandidates(tickets, 7).map((t) => t.id)).toEqual([1, 4]);
  });
  it("offers nothing to a ticket with no repo", () => {
    expect(dependencyCandidates(tickets, null)).toEqual([]);
  });
  it("groups depends-on before needed-by after the other kinds", () => {
    const groups = groupRelations([relation("depends_on", "incoming", 2), relation("depends_on", "outgoing", 1), relation("caused_by", "outgoing", 3)]);
    expect(groups.map((g) => g.label)).toEqual(["Caused by", "Depends on", "Needed by"]);
  });
  it("writes ticket references", () => {
    expect(ticketRefs([3, 5])).toBe("#3, #5");
    expect(ticketRefs([])).toBe("");
  });
  it("says which merged ticket unblocked this one", () => {
    expect(dependenciesMetText({ ticket_id: 3, merged_by_kind: "agent" })).toBe("merged #3: unblocked by #3, nothing else to wait for");
    expect(dependenciesMetText({})).toBe("merged the last dependency: nothing else to wait for");
  });
});
