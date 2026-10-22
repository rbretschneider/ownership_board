import { describe, expect, it } from "vitest";
import type { Relation, TicketSummary } from "../api/types";
import { groupRelations, relationEnds, relationLabel, searchTickets } from "./relations";

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
