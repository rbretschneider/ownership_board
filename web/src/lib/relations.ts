import type { Relation, RelationKind, TicketSummary } from "../api/types";

/** Kinds a person can choose when linking, labelled from this ticket's side. */
export const RELATION_CHOICES: readonly { kind: RelationKind; label: string }[] = [
  { kind: "caused_by", label: "Caused by" },
  { kind: "follow_up_of", label: "Follow-up of" },
  { kind: "duplicate_of", label: "Duplicate of" },
  { kind: "relates_to", label: "Relates to" },
];

/** Display label for each (kind, direction), in display order. */
const LABELS: readonly { kind: RelationKind; direction: Relation["direction"]; label: string }[] = [
  { kind: "caused_by", direction: "outgoing", label: "Caused by" },
  { kind: "caused_by", direction: "incoming", label: "Caused" },
  { kind: "follow_up_of", direction: "outgoing", label: "Follow-up of" },
  { kind: "follow_up_of", direction: "incoming", label: "Has follow-up" },
  { kind: "duplicate_of", direction: "outgoing", label: "Duplicate of" },
  { kind: "duplicate_of", direction: "incoming", label: "Duplicated by" },
  { kind: "relates_to", direction: "outgoing", label: "Relates to" },
  { kind: "relates_to", direction: "incoming", label: "Relates to" },
];

/** Most tickets the link picker lists at once. */
export const MAX_PICKER_RESULTS = 8;

/** The label for a relation as seen from this ticket. */
export function relationLabel(relation: Pick<Relation, "kind" | "direction">): string {
  return LABELS.find((l) => l.kind === relation.kind && l.direction === relation.direction)?.label ?? relation.kind;
}

/** Relations grouped under their labels, in a fixed order; empty groups are left out. */
export function groupRelations(relations: readonly Relation[]): { label: string; relations: Relation[] }[] {
  const order = [...new Set(LABELS.map((l) => l.label))];
  return order
    .map((label) => ({ label, relations: relations.filter((r) => relationLabel(r) === label) }))
    .filter((group) => group.relations.length > 0);
}

/**
 * Which side stores a relation, for DELETE /tickets/{from}/relations/{kind}/{to}:
 * an incoming relation is stored on the other ticket.
 */
export function relationEnds(ticketId: number, relation: Relation): { from: number; to: number } {
  return relation.direction === "outgoing"
    ? { from: ticketId, to: relation.ticket.id }
    : { from: relation.ticket.id, to: ticketId };
}

/**
 * Tickets matching a picker query: "#12" or "12" matches the id prefix, other
 * text matches the title. The current ticket is never offered.
 */
export function searchTickets(tickets: readonly TicketSummary[], query: string, excludeId: number): TicketSummary[] {
  const needle = query.trim().toLowerCase().replace(/^#/, "");
  const candidates = tickets.filter((t) => t.id !== excludeId);
  if (needle === "") {
    return [...candidates].sort((a, b) => b.id - a.id).slice(0, MAX_PICKER_RESULTS);
  }
  const isNumber = /^\d+$/.test(needle);
  return candidates
    .filter((t) => (isNumber && String(t.id).startsWith(needle)) || t.title.toLowerCase().includes(needle))
    .slice(0, MAX_PICKER_RESULTS);
}
