import { MoreHorizontal, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { ConfirmDialog } from "../common/ConfirmDialog";

/** Overflow actions for a ticket (currently: delete, with confirmation). */
export function TicketMenu({ ticketId, onDelete }: { ticketId: number; onDelete: () => Promise<void> }) {
  const [isOpen, setOpen] = useState(false);
  const [isConfirming, setConfirming] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen) {
      return;
    }
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current !== null && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [isOpen]);

  return (
    <div className="menu" ref={rootRef}>
      <button
        type="button"
        className="icon-button"
        aria-label="More actions"
        aria-expanded={isOpen}
        aria-controls="ticket-menu"
        onClick={() => setOpen((v) => !v)}
      >
        <MoreHorizontal size={20} aria-hidden="true" />
      </button>
      {isOpen && (
        <div id="ticket-menu" className="menu__panel">
          <button
            type="button"
            className="menu__item menu__item--danger"
            onClick={() => {
              setOpen(false);
              setConfirming(true);
            }}
          >
            <Trash2 size={16} aria-hidden="true" />
            Delete ticket
          </button>
        </div>
      )}
      {isConfirming && (
        <ConfirmDialog
          title={`Delete #${ticketId}?`}
          message="The ticket, its questions and its history are removed for good. This can't be undone."
          confirmLabel="Delete"
          onConfirm={onDelete}
          onCancel={() => setConfirming(false)}
        />
      )}
    </div>
  );
}
