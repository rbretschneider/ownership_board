import type { LaneId, TicketSummary } from "../api/types";

/*
 * Pure ordering math for drag and drop. The board keeps tickets in the order
 * the server returns them; within a lane that order is the display order.
 */

/**
 * Returns where a pointer at `pointerY` would insert a card, given the
 * vertical midpoints of the other cards in the list (top to bottom).
 */
export function insertIndexFromMidpoints(midpoints: readonly number[], pointerY: number): number {
  let index = 0;
  for (const midpoint of midpoints) {
    if (pointerY > midpoint) {
      index += 1;
    }
  }
  return index;
}

/**
 * Translates an insert position into the API's `after_id`: the card directly
 * above the drop position, or `null` for the top of the lane.
 *
 * @param laneIds Ids currently shown in the target lane, top to bottom.
 * @param draggedId The card being moved (ignored if present in `laneIds`).
 * @param insertIndex Position among the lane's other cards.
 */
export function afterIdFor(laneIds: readonly number[], draggedId: number, insertIndex: number): number | null {
  const others = laneIds.filter((id) => id !== draggedId);
  if (insertIndex <= 0 || others.length === 0) {
    return null;
  }
  const clamped = Math.min(insertIndex, others.length);
  return others[clamped - 1] ?? null;
}

/**
 * Applies a move optimistically: puts the ticket into `lane` directly after
 * `afterId`, or at the top of the lane when `afterId` is null.
 * Returns a new array; the input is not modified.
 */
export function applyMove(
  tickets: readonly TicketSummary[],
  ticketId: number,
  lane: LaneId,
  afterId: number | null,
): TicketSummary[] {
  const moving = tickets.find((t) => t.id === ticketId);
  if (moving === undefined) {
    return [...tickets];
  }
  const rest = tickets.filter((t) => t.id !== ticketId);
  const moved: TicketSummary = { ...moving, lane };
  rest.splice(insertionPoint(rest, lane, afterId), 0, moved);
  return rest;
}

/** Groups tickets by lane, preserving their relative order. */
export function groupByLane(
  tickets: readonly TicketSummary[],
  laneIds: readonly LaneId[],
): Map<LaneId, TicketSummary[]> {
  const groups = new Map<LaneId, TicketSummary[]>(laneIds.map((id) => [id, []]));
  for (const ticket of tickets) {
    groups.get(ticket.lane)?.push(ticket);
  }
  return groups;
}

function insertionPoint(rest: readonly TicketSummary[], lane: LaneId, afterId: number | null): number {
  if (afterId !== null) {
    const afterIndex = rest.findIndex((t) => t.id === afterId);
    if (afterIndex >= 0) {
      return afterIndex + 1;
    }
  }
  const firstInLane = rest.findIndex((t) => t.lane === lane);
  return firstInLane >= 0 ? firstInLane : rest.length;
}
