import { MoreHorizontal } from "lucide-react";
import { useEffect, useId, useRef, useState, type ComponentType } from "react";

/** One action in a {@link RowMenu}. */
export interface RowMenuItem {
  label: string;
  icon: ComponentType<{ size?: number; "aria-hidden"?: boolean | "true" }>;
  onSelect: () => void;
  isDanger?: boolean;
}

/** A small overflow menu for a list row. `label` names the row for screen readers. */
export function RowMenu({ label, items }: { label: string; items: RowMenuItem[] }) {
  const [isOpen, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const panelId = useId();

  useEffect(() => {
    if (!isOpen) {
      return;
    }
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current !== null && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [isOpen]);

  return (
    <div className="menu" ref={rootRef}>
      <button
        type="button"
        className="icon-button icon-button--small"
        aria-label={`Actions for ${label}`}
        aria-expanded={isOpen}
        aria-controls={panelId}
        onClick={() => setOpen((v) => !v)}
      >
        <MoreHorizontal size={18} aria-hidden="true" />
      </button>
      {isOpen && (
        <div id={panelId} className="menu__panel">
          {items.map(({ label: itemLabel, icon: Icon, onSelect, isDanger }) => (
            <button
              key={itemLabel}
              type="button"
              className={`menu__item${isDanger === true ? " menu__item--danger" : ""}`}
              onClick={() => {
                setOpen(false);
                onSelect();
              }}
            >
              <Icon size={16} aria-hidden="true" />
              {itemLabel}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
