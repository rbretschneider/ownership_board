import { ExternalLink, X } from "lucide-react";
import { nodeStatus, retries, type FlowFocus, type FlowLayout } from "../../lib/flow";
import { ticketHash } from "../../lib/hashRoute";

/** Props for {@link FlowFocusBar}. */
export interface FlowFocusBarProps {
  layout: FlowLayout;
  focus: FlowFocus | null;
  now: number;
  onClear: () => void;
}

function tickets(n: number): string {
  return `${n} ticket${n === 1 ? "" : "s"}`;
}

/** What the focused ticket waits on, what it unlocks, and the chain it is stuck behind. */
export function FlowFocusBar({ layout, focus, now, onClear }: FlowFocusBarProps) {
  if (focus === null) {
    return (
      <p className="flow-focus flow-focus--hint">
        Lines run from a ticket to the work it waits on. Pick a ticket to treat it as a milestone: what must land first
        and what it unlocks stay lit. Esc clears.
      </p>
    );
  }
  const ticket = layout.graph.byId.get(focus.id);
  if (ticket === undefined) return null;
  const head = focus.path.length > 1 ? layout.graph.byId.get(focus.path[focus.path.length - 1] as number) : undefined;
  const status = nodeStatus(ticket, now);
  const loops = retries(ticket);
  return (
    <section className="flow-focus" aria-live="polite" aria-label={`Focused on #${ticket.id}`}>
      <div className="flow-focus__main">
        <p className="flow-focus__title">
          <span className={`flow-dot flow-dot--${status.tone}`} aria-hidden="true" />
          <strong>#{ticket.id}</strong> {ticket.title}
        </p>
        <p className="flow-focus__facts">
          <span>{status.label}</span>
          {focus.upstream.size > 0
            ? <span>{tickets(focus.upstream.size)} must land first</span>
            : <span>Nothing to wait for</span>}
          {head !== undefined && (
            <span>
              longest chain {focus.path.length - 1} hand-off{focus.path.length === 2 ? "" : "s"}, stuck behind{" "}
              <strong>#{head.id}</strong> ({nodeStatus(head, now).label})
            </span>
          )}
          <span>{focus.downstream.size > 0 ? `unlocks ${tickets(focus.downstream.size)}` : "unlocks nothing yet"}</span>
          {loops.total > 0 && <span>{loops.detail}</span>}
        </p>
      </div>
      <a className="button button--ghost flow-focus__open" href={ticketHash(ticket.id)}>
        <ExternalLink size={16} aria-hidden="true" /> Open ticket
      </a>
      <button type="button" className="icon-button" onClick={onClear} aria-label="Clear focus" title="Clear (Esc)">
        <X size={18} aria-hidden="true" />
      </button>
    </section>
  );
}
