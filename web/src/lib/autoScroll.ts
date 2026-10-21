/** Distance from a scroll container's edge where auto-scroll kicks in. */
const EDGE_PX = 56;
/** Fastest auto-scroll speed, in pixels per animation frame. */
const MAX_SPEED_PX = 18;

const SCROLLABLE = new Set(["auto", "scroll"]);

/**
 * Speed (negative = towards the start) for a pointer at `position` inside the
 * span `[start, end]`: zero away from the edges, ramping up near them.
 */
export function edgeSpeed(position: number, start: number, end: number): number {
  const fromStart = position - start;
  const fromEnd = end - position;
  if (fromStart < EDGE_PX) {
    return -Math.round(MAX_SPEED_PX * (1 - Math.max(0, fromStart) / EDGE_PX));
  }
  if (fromEnd < EDGE_PX) {
    return Math.round(MAX_SPEED_PX * (1 - Math.max(0, fromEnd) / EDGE_PX));
  }
  return 0;
}

/**
 * Scrolls every scrollable ancestor of the element under the pointer (and the
 * page) when the pointer is near its edge. Returns true if anything scrolled.
 */
export function autoScrollAt(x: number, y: number): boolean {
  let didScroll = false;
  for (const container of scrollContainersAt(x, y)) {
    const isPage = container === document.scrollingElement;
    const rect = isPage ? new DOMRect(0, 0, window.innerWidth, window.innerHeight) : container.getBoundingClientRect();
    const dy = canScroll(container, "y") ? edgeSpeed(y, rect.top, rect.bottom) : 0;
    const dx = canScroll(container, "x") ? edgeSpeed(x, rect.left, rect.right) : 0;
    if (dx !== 0 || dy !== 0) {
      const beforeTop = container.scrollTop;
      const beforeLeft = container.scrollLeft;
      container.scrollBy(dx, dy);
      didScroll = didScroll || container.scrollTop !== beforeTop || container.scrollLeft !== beforeLeft;
    }
  }
  return didScroll;
}

function scrollContainersAt(x: number, y: number): Element[] {
  const containers: Element[] = [];
  let node: Element | null = document.elementFromPoint(x, y);
  while (node !== null && node !== document.documentElement && node !== document.body) {
    if (canScroll(node, "y") || canScroll(node, "x")) {
      containers.push(node);
    }
    node = node.parentElement;
  }
  if (document.scrollingElement !== null) {
    containers.push(document.scrollingElement);
  }
  return containers;
}

function canScroll(el: Element, axis: "x" | "y"): boolean {
  const isPage = el === document.scrollingElement;
  const overflowsContent = axis === "y" ? el.scrollHeight > el.clientHeight : el.scrollWidth > el.clientWidth;
  if (isPage || !overflowsContent) {
    return overflowsContent;
  }
  const style = getComputedStyle(el);
  return SCROLLABLE.has(axis === "y" ? style.overflowY : style.overflowX);
}
