import { Globe, Plus, X } from "lucide-react";
import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Lane, Relation, TicketDetail, TicketSummary } from "../../api/types";
import { ticketHash } from "../../lib/hashRoute";
import { groupRelations, relationEnds, relationLabel, ticketRefs } from "../../lib/relations";
import { ConfirmDialog } from "../common/ConfirmDialog";
import { RelationForm } from "./RelationForm";
import type { RunMutation } from "./TicketSheet";

/** Props for {@link RelatedSection}. */
export interface RelatedSectionProps {
  ticket: TicketDetail;
  tickets: readonly TicketSummary[];
  lanes: readonly Lane[];
  run: RunMutation;
}

/** Typed links to other tickets ("caused by #42"), grouped by how they relate. */
export function RelatedSection({ ticket, tickets, lanes, run }: RelatedSectionProps) {
  const api = useApi();
  const [isAdding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Relation | null>(null);
  const groups = groupRelations(ticket.relations);
  const laneLabel = (id: string) => lanes.find((l) => l.id === id)?.label ?? id;

  // An incoming relation is stored on the other ticket, so it is deleted there;
  // this ticket is then reloaded to show the change.
  const remove = async (relation: Relation) => {
    const { from, to } = relationEnds(ticket.id, relation);
    await api.removeRelation(from, relation.kind, to);
    setRemoving(null);
    await run(() => api.ticket(ticket.id), "Link removed");
  };

  return (
    <section className="section" aria-labelledby="related-title">
      <div className="section__head">
        <h3 id="related-title" className="section__title">Related</h3>
        {!isAdding && (
          <button type="button" className="button button--ghost button--small" onClick={() => setAdding(true)}>
            <Plus size={14} aria-hidden="true" />
            Link ticket
          </button>
        )}
      </div>
      {isAdding && (
        <RelationForm
          ticketId={ticket.id}
          repoId={ticket.repo_id}
          tickets={tickets}
          onSubmit={(input) => run(() => api.addRelation(ticket.id, input), "Tickets linked")}
          onCancel={() => setAdding(false)}
        />
      )}
      {ticket.waiting_on.length > 0 && (
        <p className="related-waiting" role="status">
          Waiting on {ticketRefs(ticket.waiting_on)}: agents cannot claim this until {ticket.waiting_on.length === 1 ? "it is" : "they are"} merged.
        </p>
      )}
      {groups.length === 0 && !isAdding && <p className="muted">Not linked to other tickets.</p>}
      {groups.map((group) => (
        <div key={group.label} className="related-group">
          <h4 className="related-group__label">{group.label}</h4>
          <ul className="related-list">
            {group.relations.map((relation) => (
              <li key={`${relation.kind}-${relation.direction}-${relation.ticket.id}`} className="related-item">
                <a className="related-item__link" href={ticketHash(relation.ticket.id)}>
                  <span className="related-item__id">#{relation.ticket.id}</span> {relation.ticket.title}
                </a>
                <span className="badge">{laneLabel(relation.ticket.lane)}</span>
                {relation.ticket.visibility === "public" && (
                  <span className="badge badge--public" title="That ticket's repo is public">
                    <Globe size={12} aria-hidden="true" />
                    public repo
                  </span>
                )}
                <button
                  type="button"
                  className="icon-button icon-button--small"
                  aria-label={`Remove link: ${relationLabel(relation)} #${relation.ticket.id}`}
                  onClick={() => setRemoving(relation)}
                >
                  <X size={16} aria-hidden="true" />
                </button>
              </li>
            ))}
          </ul>
        </div>
      ))}
      {removing !== null && (
        <ConfirmDialog
          title={`Unlink #${removing.ticket.id}?`}
          message={`“${relationLabel(removing)} #${removing.ticket.id}” is removed from both tickets. The tickets themselves stay.`}
          confirmLabel="Unlink"
          onConfirm={() => remove(removing)}
          onCancel={() => setRemoving(null)}
        />
      )}
    </section>
  );
}
