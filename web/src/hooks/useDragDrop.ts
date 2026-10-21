import { useCallback, useEffect, useLayoutEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type RefObject } from "react";
import { autoScrollAt } from "../lib/autoScroll";
import { findDropTarget, isSameTarget, type DropTarget } from "../lib/dropTargets";

/** Mouse/pen movement needed before a press becomes a drag. */
const MOUSE_DRAG_THRESHOLD_PX = 5;
/** Touch hold time before a press becomes a drag (shorter holds scroll). */
const TOUCH_LONG_PRESS_MS = 300;
/** Touch movement that cancels a pending long-press (the user is scrolling). */
const TOUCH_MOVE_TOLERANCE_PX = 8;
/** Haptic tick when a touch drag starts. */
const DRAG_START_VIBRATE_MS = 10;
/** How long a click is ignored after a drop. */
const CLICK_SUPPRESS_MS = 400;

const PRESSING_CLASS = "is-pressing";
const BODY_DRAGGING_CLASS = "is-dragging";

/** A drag in progress, as the UI needs to render it. */
export interface DragSession {
  ticketId: number;
  width: number;
  target: DropTarget | null;
}

/** What {@link useDragDrop} gives the board. */
export interface DragDrop {
  session: DragSession | null;
  /** Attach to the floating ghost element; the hook positions it. */
  ghostRef: RefObject<HTMLDivElement | null>;
  /** Call from a card's `onPointerDown`. */
  startPress: (event: ReactPointerEvent<HTMLElement>, ticketId: number) => void;
  /** True (once) if a click should be ignored because it ended a drag. */
  consumeClick: () => boolean;
}

interface Press {
  pointerId: number;
  pointerType: string;
  startX: number;
  startY: number;
  ticketId: number;
  element: HTMLElement;
  timer: number | null;
}

interface Active {
  ticketId: number;
  element: HTMLElement;
  offsetX: number;
  offsetY: number;
  x: number;
  y: number;
  target: DropTarget | null;
}

/**
 * Pointer Events drag and drop for cards: mouse, pen and touch (long-press).
 * Drop targets are found by hit-testing elements marked with the attributes in
 * `lib/dropTargets`, so lanes and pills need no registration.
 */
export function useDragDrop(onDrop: (ticketId: number, target: DropTarget) => void): DragDrop {
  const [session, setSession] = useState<DragSession | null>(null);
  const ghostRef = useRef<HTMLDivElement | null>(null);
  const press = useRef<Press | null>(null);
  const active = useRef<Active | null>(null);
  const frame = useRef<number | null>(null);
  const suppressUntil = useRef(0);
  const onDropRef = useRef(onDrop);
  onDropRef.current = onDrop;

  const positionGhost = useCallback(() => {
    const drag = active.current;
    const ghost = ghostRef.current;
    if (drag !== null && ghost !== null) {
      ghost.style.transform = `translate3d(${drag.x - drag.offsetX}px, ${drag.y - drag.offsetY}px, 0)`;
    }
  }, []);

  const retarget = useCallback(() => {
    const drag = active.current;
    if (drag === null) {
      return;
    }
    const target = findDropTarget(drag.x, drag.y, drag.ticketId);
    if (!isSameTarget(target, drag.target)) {
      drag.target = target;
      setSession((current) => (current === null ? null : { ...current, target }));
    }
  }, []);

  const tick = useCallback(() => {
    const drag = active.current;
    if (drag === null) {
      frame.current = null;
      return;
    }
    if (autoScrollAt(drag.x, drag.y)) {
      retarget();
    }
    frame.current = requestAnimationFrame(tick);
  }, [retarget]);

  const clearPress = useCallback(() => {
    const pending = press.current;
    if (pending !== null) {
      if (pending.timer !== null) {
        window.clearTimeout(pending.timer);
      }
      pending.element.classList.remove(PRESSING_CLASS);
    }
    press.current = null;
  }, []);

  const finish = useCallback((shouldDrop: boolean) => {
    const drag = active.current;
    clearPress();
    if (drag === null) {
      return;
    }
    active.current = null;
    if (frame.current !== null) {
      cancelAnimationFrame(frame.current);
      frame.current = null;
    }
    drag.element.style.touchAction = "";
    document.body.classList.remove(BODY_DRAGGING_CLASS);
    suppressUntil.current = Date.now() + CLICK_SUPPRESS_MS;
    setSession(null);
    if (shouldDrop && drag.target !== null) {
      onDropRef.current(drag.ticketId, drag.target);
    }
  }, [clearPress]);

  const begin = useCallback((x: number, y: number) => {
    const pending = press.current;
    if (pending === null) {
      return;
    }
    const rect = pending.element.getBoundingClientRect();
    active.current = {
      ticketId: pending.ticketId,
      element: pending.element,
      offsetX: pending.startX - rect.left,
      offsetY: pending.startY - rect.top,
      x,
      y,
      target: null,
    };
    pending.element.style.touchAction = "none";
    if (pending.pointerType === "touch") {
      navigator.vibrate?.(DRAG_START_VIBRATE_MS);
    }
    clearPress();
    document.body.classList.add(BODY_DRAGGING_CLASS);
    setSession({ ticketId: active.current.ticketId, width: rect.width, target: null });
    retarget();
    frame.current = requestAnimationFrame(tick);
  }, [clearPress, retarget, tick]);

  const startPress = useCallback((event: ReactPointerEvent<HTMLElement>, ticketId: number) => {
    if (active.current !== null || !isPrimaryPress(event)) {
      return;
    }
    clearPress();
    const element = event.currentTarget;
    const pending: Press = {
      pointerId: event.pointerId,
      pointerType: event.pointerType,
      startX: event.clientX,
      startY: event.clientY,
      ticketId,
      element,
      timer: null,
    };
    if (event.pointerType === "touch") {
      element.classList.add(PRESSING_CLASS);
      pending.timer = window.setTimeout(() => begin(pending.startX, pending.startY), TOUCH_LONG_PRESS_MS);
    }
    press.current = pending;
  }, [begin, clearPress]);

  useLayoutEffect(positionGhost, [session?.ticketId, positionGhost]);

  useEffect(() => {
    const onMove = (event: PointerEvent) => {
      const drag = active.current;
      if (drag !== null) {
        drag.x = event.clientX;
        drag.y = event.clientY;
        positionGhost();
        retarget();
        return;
      }
      handlePendingMove(event);
    };
    const handlePendingMove = (event: PointerEvent) => {
      const pending = press.current;
      if (pending === null || pending.pointerId !== event.pointerId) {
        return;
      }
      const distance = Math.hypot(event.clientX - pending.startX, event.clientY - pending.startY);
      if (pending.pointerType === "touch") {
        if (distance > TOUCH_MOVE_TOLERANCE_PX) {
          clearPress();
        }
      } else if (distance > MOUSE_DRAG_THRESHOLD_PX) {
        begin(event.clientX, event.clientY);
      }
    };
    const onUp = () => finish(true);
    const onCancel = () => finish(false);
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && active.current !== null) {
        event.preventDefault();
        finish(false);
      }
    };
    // Once a touch drag has started, stop the page from scrolling under it.
    const onTouchMove = (event: TouchEvent) => {
      if (active.current !== null) {
        event.preventDefault();
      }
    };
    const onContextMenu = (event: Event) => {
      if (active.current !== null || press.current?.pointerType === "touch") {
        event.preventDefault();
      }
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onCancel);
    window.addEventListener("keydown", onKey);
    window.addEventListener("touchmove", onTouchMove, { passive: false });
    window.addEventListener("contextmenu", onContextMenu);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onCancel);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("touchmove", onTouchMove);
      window.removeEventListener("contextmenu", onContextMenu);
    };
  }, [begin, clearPress, finish, positionGhost, retarget]);

  const consumeClick = useCallback(() => {
    if (Date.now() < suppressUntil.current) {
      suppressUntil.current = 0;
      return true;
    }
    return false;
  }, []);

  return { session, ghostRef, startPress, consumeClick };
}

function isPrimaryPress(event: ReactPointerEvent<HTMLElement>): boolean {
  if (event.pointerType === "mouse" && event.button !== 0) {
    return false;
  }
  return !event.ctrlKey && !event.metaKey && !event.shiftKey;
}
