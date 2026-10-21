import { ExternalLink, GitBranch, GitPullRequest, Pencil } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useApi } from "../../api/ApiContext";
import type { TicketDetail } from "../../api/types";
import { Spinner } from "../common/Spinner";
import type { RunMutation } from "./TicketSheet";

/** PR link and branch, with a small inline editor. */
export function PrRow({ ticket, run }: { ticket: TicketDetail; run: RunMutation }) {
  const api = useApi();
  const [isEditing, setEditing] = useState(false);
  const [prUrl, setPrUrl] = useState(ticket.pr_url);
  const [branch, setBranch] = useState(ticket.branch);
  const [isBusy, setBusy] = useState(false);

  const startEditing = () => {
    setPrUrl(ticket.pr_url);
    setBranch(ticket.branch);
    setEditing(true);
  };

  const save = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    const ok = await run(() => api.editTicket(ticket.id, { pr_url: prUrl.trim(), branch: branch.trim() }), "Saved");
    setBusy(false);
    if (ok) {
      setEditing(false);
    }
  };

  if (isEditing) {
    return (
      <form className="pr-row pr-row--editing" onSubmit={(e) => void save(e)}>
        <div className="field">
          <label className="field__label" htmlFor="pr-url">Pull request URL</label>
          <input id="pr-url" className="input" type="url" inputMode="url" value={prUrl} onChange={(e) => setPrUrl(e.target.value)} autoFocus />
        </div>
        <div className="field">
          <label className="field__label" htmlFor="pr-branch">Branch</label>
          <input id="pr-branch" className="input" value={branch} onChange={(e) => setBranch(e.target.value)} spellCheck={false} />
        </div>
        <div className="row-actions">
          <button type="button" className="button button--secondary" onClick={() => setEditing(false)}>Cancel</button>
          <button type="submit" className="button button--primary" disabled={isBusy}>
            {isBusy && <Spinner label="Saving" />}
            Save
          </button>
        </div>
      </form>
    );
  }

  const hasAny = ticket.pr_url !== "" || ticket.branch !== "";
  return (
    <div className="pr-row">
      {ticket.pr_url !== "" && (
        <a className="pr-row__link" href={ticket.pr_url} target="_blank" rel="noopener noreferrer">
          <GitPullRequest size={16} aria-hidden="true" />
          Pull request
          <ExternalLink size={14} aria-hidden="true" />
        </a>
      )}
      {ticket.branch !== "" && (
        <code className="pr-row__branch">
          <GitBranch size={14} aria-hidden="true" />
          {ticket.branch}
        </code>
      )}
      {!hasAny && <span className="muted">No PR yet.</span>}
      <button type="button" className="button button--ghost button--small" onClick={startEditing}>
        <Pencil size={14} aria-hidden="true" />
        {hasAny ? "Edit" : "Add PR link"}
      </button>
    </div>
  );
}
