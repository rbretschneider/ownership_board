import { CircleAlert, Play } from "lucide-react";
import { useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { TicketDetail } from "../../api/types";
import { AutosizeTextarea } from "../common/AutosizeTextarea";
import { Spinner } from "../common/Spinner";
import type { RunMutation } from "./TicketSheet";

/** A blocked ticket: the agent's reason (plain text, never markdown) and Unblock. */
export function BlockedPanel({ ticket, run }: { ticket: TicketDetail; run: RunMutation }) {
  const api = useApi();
  const id = useId();
  const [comment, setComment] = useState("");
  const [isBusy, setBusy] = useState(false);

  const unblock = async () => {
    setBusy(true);
    const ok = await run(() => api.unblock(ticket.id, comment.trim()), "Unblocked");
    if (!ok) {
      setBusy(false);
    }
  };

  return (
    <section className="blocked" aria-labelledby={`${id}-title`}>
      <header className="blocked__header">
        <CircleAlert size={20} aria-hidden="true" />
        <h3 id={`${id}-title`} className="blocked__title">Blocked</h3>
      </header>
      <p className="blocked__reason">{ticket.blocked_reason}</p>
      <div className="field">
        <label className="field__label" htmlFor={`${id}-comment`}>Comment (optional)</label>
        <AutosizeTextarea id={`${id}-comment`} value={comment} onChange={(e) => setComment(e.target.value)} rows={2} />
      </div>
      <div className="row-actions">
        <button type="button" className="button button--primary" onClick={() => void unblock()} disabled={isBusy}>
          {isBusy ? <Spinner label="Unblocking" /> : <Play size={16} aria-hidden="true" />}
          Unblock
        </button>
      </div>
    </section>
  );
}
