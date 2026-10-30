import { useId, useState, type FormEvent } from "react";
import type { RelationInput, RelationKind, TicketSummary } from "../../api/types";
import { dependencyCandidates, RELATION_CHOICES, searchTickets } from "../../lib/relations";
import { Spinner } from "../common/Spinner";

/** Props for {@link RelationForm}. */
export interface RelationFormProps {
  ticketId: number;
  /** This ticket's repo; a dependency must be in the same one. */
  repoId: number | null;
  tickets: readonly TicketSummary[];
  onSubmit: (input: RelationInput) => Promise<boolean>;
  onCancel: () => void;
}

/** Links this ticket to another: choose how they relate, then pick the ticket. */
export function RelationForm({ ticketId, repoId, tickets, onSubmit, onCancel }: RelationFormProps) {
  const id = useId();
  const [kind, setKind] = useState<RelationKind>("relates_to");
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<TicketSummary | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);
  const isDependency = kind === "depends_on";
  const choosable = isDependency ? dependencyCandidates(tickets, repoId) : tickets;
  const results = picked === null ? searchTickets(choosable, query, ticketId) : [];

  // A ticket picked for another kind may be in a different repo.
  const changeKind = (next: RelationKind) => {
    setKind(next);
    if (next === "depends_on" && picked !== null && dependencyCandidates([picked], repoId).length === 0) {
      setPicked(null);
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (picked === null) {
      setProblem("Pick the ticket to link to.");
      return;
    }
    setProblem(null);
    setBusy(true);
    const ok = await onSubmit({ kind, ticket_id: picked.id });
    setBusy(false);
    if (ok) {
      onCancel();
    }
  };

  return (
    <form className="inline-form" onSubmit={(e) => void submit(e)} noValidate>
      <div className="field">
        <label className="field__label" htmlFor={`${id}-kind`}>This ticket is</label>
        <select id={`${id}-kind`} className="select" value={kind} onChange={(e) => changeKind(e.target.value as RelationKind)}>
          {RELATION_CHOICES.map((choice) => (
            <option key={choice.kind} value={choice.kind}>{choice.label}</option>
          ))}
        </select>
        {isDependency && (
          <p className="muted small">
            {repoId === null
              ? "Set this ticket's repo first: dependencies are same repo only."
              : "Agents cannot claim this ticket until the one you pick is merged. Same repo only."}
          </p>
        )}
      </div>
      <div className="field">
        <label className="field__label" htmlFor={`${id}-search`}>Ticket</label>
        {picked === null ? (
          <>
            <input
              id={`${id}-search`}
              className="input"
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="#id or words from the title"
              autoComplete="off"
              aria-controls={`${id}-results`}
              autoFocus
            />
            <ul id={`${id}-results`} className="ticket-picker" aria-label="Matching tickets">
              {results.map((candidate) => (
                <li key={candidate.id}>
                  <button type="button" className="ticket-picker__option" onClick={() => setPicked(candidate)}>
                    <span className="ticket-picker__id">#{candidate.id}</span> {candidate.title}
                  </button>
                </li>
              ))}
              {results.length === 0 && <li className="muted small">No tickets match.</li>}
            </ul>
          </>
        ) : (
          <div className="ticket-picker__picked">
            <span><span className="ticket-picker__id">#{picked.id}</span> {picked.title}</span>
            <button type="button" className="button button--ghost button--small" onClick={() => setPicked(null)}>Change</button>
          </div>
        )}
      </div>
      {problem !== null && <p className="form-error" role="alert">{problem}</p>}
      <div className="row-actions">
        <button type="button" className="button button--secondary" onClick={onCancel}>Cancel</button>
        <button type="submit" className="button button--primary" disabled={isBusy}>
          {isBusy && <Spinner label="Linking" />}
          Link ticket
        </button>
      </div>
    </form>
  );
}
