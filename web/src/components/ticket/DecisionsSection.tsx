import { Plus } from "lucide-react";
import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Decision, TicketDetail } from "../../api/types";
import { deleteMessage, groupDecisions } from "../../lib/decisions";
import { ConfirmDialog } from "../common/ConfirmDialog";
import { DecisionForm } from "./DecisionForm";
import { DecisionRow } from "./DecisionRow";
import type { RunMutation } from "./TicketSheet";

type FormState = { mode: "closed" } | { mode: "add" } | { mode: "replace"; decision: Decision };

/** The ticket's decision log: active decisions, then a collapsed history. */
export function DecisionsSection({ ticket, run }: { ticket: TicketDetail; run: RunMutation }) {
  const api = useApi();
  const [form, setForm] = useState<FormState>({ mode: "closed" });
  const [deleting, setDeleting] = useState<Decision | null>(null);
  const { active, history } = groupDecisions(ticket.decisions);

  // The API call runs inside the confirm dialog so a failure shows there, in
  // the server's words, and the dialog stays open for a retry.
  const remove = async (decision: Decision) => {
    const updated = await api.removeDecision(ticket.id, decision.id);
    setDeleting(null);
    await run(() => Promise.resolve(updated), "Decision removed");
  };

  return (
    <section className="section" aria-labelledby="decisions-title">
      <div className="section__head">
        <h3 id="decisions-title" className="section__title">
          Decisions <span className="section__hint">yours are binding on the AI</span>
        </h3>
        {form.mode === "closed" && (
          <button type="button" className="button button--ghost button--small" onClick={() => setForm({ mode: "add" })}>
            <Plus size={14} aria-hidden="true" />
            Add decision
          </button>
        )}
      </div>
      {form.mode !== "closed" && (
        <DecisionForm
          replacing={form.mode === "replace" ? form.decision : null}
          onSubmit={(input) => run(() => api.addDecision(ticket.id, input), form.mode === "replace" ? "Decision replaced" : "Decision recorded")}
          onCancel={() => setForm({ mode: "closed" })}
        />
      )}
      {active.length === 0 && form.mode === "closed" && <p className="muted">No decisions recorded yet.</p>}
      {active.length > 0 && (
        <ul className="decision-list">
          {active.map((decision) => (
            <DecisionRow
              key={decision.id}
              decision={decision}
              all={ticket.decisions}
              questions={ticket.questions}
              onReplace={() => setForm({ mode: "replace", decision })}
              onDelete={() => setDeleting(decision)}
            />
          ))}
        </ul>
      )}
      {history.length > 0 && (
        <details className="decision-history">
          <summary>{history.length === 1 ? "1 earlier decision" : `${history.length} earlier decisions`}</summary>
          <ul className="decision-list">
            {history.map((decision) => (
              <DecisionRow key={decision.id} decision={decision} all={ticket.decisions} questions={ticket.questions} />
            ))}
          </ul>
        </details>
      )}
      {deleting !== null && (
        <ConfirmDialog
          title="Delete this decision?"
          message={deleteMessage(deleting)}
          confirmLabel="Delete"
          onConfirm={() => remove(deleting)}
          onCancel={() => setDeleting(null)}
        />
      )}
    </section>
  );
}
