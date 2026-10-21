import { useEffect, useRef, type KeyboardEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";

/** Layout of an overlay surface. */
export type OverlayVariant =
  /** Centered dialog on desktop, full-height bottom sheet on phones. */
  | "dialog"
  /** Right-hand panel on desktop, full screen on phones. */
  | "panel"
  /** Small centered alert on every screen size. */
  | "alert";

/** Props for {@link Overlay}. */
export interface OverlayProps {
  variant: OverlayVariant;
  /** Accessible name of the dialog. */
  label: string;
  onClose: () => void;
  children: ReactNode;
  /** Use `alertdialog` semantics (confirmations). */
  isAlert?: boolean;
}

const FOCUSABLE = [
  "a[href]", "button:not([disabled])", "input:not([disabled])", "select:not([disabled])",
  "textarea:not([disabled])", "[tabindex]:not([tabindex='-1'])",
].join(",");

/**
 * A modal surface rendered into `document.body`: traps focus, closes on Escape
 * or a backdrop click, locks page scroll, and returns focus when it closes.
 */
export function Overlay({ variant, label, onClose, children, isAlert = false }: OverlayProps) {
  const surfaceRef = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    focusFirst(surfaceRef.current);
    return () => {
      document.body.style.overflow = previousOverflow;
      previouslyFocused?.focus?.({ preventScroll: true });
    };
  }, []);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      // Stop here so an overlay underneath (React bubbles through portals) stays open.
      event.stopPropagation();
      onCloseRef.current();
      return;
    }
    if (event.key === "Tab") {
      trapTab(event, surfaceRef.current);
    }
  };

  return createPortal(
    <div className={`overlay overlay--${variant}`} onKeyDown={onKeyDown}>
      <div className="overlay__backdrop" aria-hidden="true" onClick={() => onCloseRef.current()} />
      <div
        ref={surfaceRef}
        className={`overlay__surface overlay__surface--${variant}`}
        role={isAlert ? "alertdialog" : "dialog"}
        aria-modal="true"
        aria-label={label}
      >
        {children}
      </div>
    </div>,
    document.body,
  );
}

function focusFirst(surface: HTMLElement | null): void {
  if (surface === null) {
    return;
  }
  const preferred = surface.querySelector<HTMLElement>("[data-autofocus]");
  const target = preferred ?? surface.querySelector<HTMLElement>(FOCUSABLE) ?? surface;
  if (target === surface) {
    surface.tabIndex = -1;
  }
  target.focus({ preventScroll: true });
}

function trapTab(event: KeyboardEvent, surface: HTMLElement | null): void {
  if (surface === null) {
    return;
  }
  const focusables = [...surface.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => el.offsetParent !== null);
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  if (first === undefined || last === undefined) {
    event.preventDefault();
    return;
  }
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}
