import type { LaneId } from "../api/types";
import { insertIndexFromMidpoints } from "./position";

/** Attribute marking a lane list that accepts drops (value: lane id). */
export const DROP_LIST_ATTR = "data-drop-list";
/** Attribute marking a mobile lane pill that accepts drops (value: lane id). */
export const DROP_PILL_ATTR = "data-drop-pill";
/** Attribute carrying a card's ticket id. */
export const CARD_ATTR = "data-card-id";

/** Where a dragged card would land. */
export type DropTarget =
  | { kind: "list"; lane: LaneId; index: number }
  | { kind: "pill"; lane: LaneId };

/** Finds the drop target under a viewport point, ignoring the dragged card. */
export function findDropTarget(x: number, y: number, draggedId: number): DropTarget | null {
  const hit = document.elementFromPoint(x, y);
  if (hit === null) {
    return null;
  }
  const pill = hit.closest<HTMLElement>(`[${DROP_PILL_ATTR}]`);
  if (pill !== null) {
    return { kind: "pill", lane: pill.getAttribute(DROP_PILL_ATTR) as LaneId };
  }
  const list = hit.closest<HTMLElement>(`[${DROP_LIST_ATTR}]`);
  if (list === null) {
    return null;
  }
  const midpoints = cardsIn(list, draggedId).map((card) => {
    const rect = card.getBoundingClientRect();
    return rect.top + rect.height / 2;
  });
  return { kind: "list", lane: list.getAttribute(DROP_LIST_ATTR) as LaneId, index: insertIndexFromMidpoints(midpoints, y) };
}

/** Ticket ids shown in a lane list, top to bottom (DOM order). */
export function laneCardIds(lane: LaneId): number[] {
  const list = document.querySelector<HTMLElement>(`[${DROP_LIST_ATTR}="${lane}"]`);
  if (list === null) {
    return [];
  }
  return cardsIn(list, Number.NaN).map((card) => Number(card.getAttribute(CARD_ATTR)));
}

/** True when two targets are the same place. */
export function isSameTarget(a: DropTarget | null, b: DropTarget | null): boolean {
  if (a === null || b === null) {
    return a === b;
  }
  if (a.kind === "list" && b.kind === "list") {
    return a.lane === b.lane && a.index === b.index;
  }
  return a.kind === b.kind && a.lane === b.lane;
}

function cardsIn(list: HTMLElement, excludeId: number): HTMLElement[] {
  return [...list.querySelectorAll<HTMLElement>(`[${CARD_ATTR}]`)].filter(
    (card) => Number(card.getAttribute(CARD_ATTR)) !== excludeId,
  );
}
