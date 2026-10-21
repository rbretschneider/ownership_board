import { ExternalLink, GitCommitHorizontal, GitPullRequest, Plus, X } from "lucide-react";
import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { TicketDetail, WorkLink } from "../../api/types";
import { relativeTime } from "../../lib/relativeTime";
import { linkHost, shortSha, urlPathText } from "../../lib/workLinks";
import { ConfirmDialog } from "../common/ConfirmDialog";
import type { RunMutation } from "./TicketSheet";
import { WorkLinkForm } from "./WorkLinkForm";

/** The PRs and commits that implement the ticket. */
export function WorkSection({ ticket, run }: { ticket: TicketDetail; run: RunMutation }) {
  const api = useApi();
  const [isAdding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<WorkLink | null>(null);

  const remove = async (link: WorkLink) => {
    const updated = await api.removeWorkLink(ticket.id, link.id);
    setRemoving(null);
    await run(() => Promise.resolve(updated), "Link removed");
  };

  return (
    <section className="section" aria-labelledby="work-title">
      <div className="section__head">
        <h3 id="work-title" className="section__title">Work <span className="section__hint">PRs and commits</span></h3>
        {!isAdding && (
          <button type="button" className="button button--ghost button--small" onClick={() => setAdding(true)}>
            <Plus size={14} aria-hidden="true" />
            Add link
          </button>
        )}
      </div>
      {isAdding && (
        <WorkLinkForm
          onSubmit={(input) => run(() => api.addWorkLink(ticket.id, input), "Link added")}
          onCancel={() => setAdding(false)}
        />
      )}
      {ticket.work_links.length === 0 && !isAdding && <p className="muted">No PRs or commits linked yet.</p>}
      {ticket.work_links.length > 0 && (
        <ul className="work-list">
          {ticket.work_links.map((link) => (
            <WorkLinkRow key={link.id} link={link} onRemove={() => setRemoving(link)} />
          ))}
        </ul>
      )}
      {removing !== null && (
        <ConfirmDialog
          title="Remove this link?"
          message="The PR or commit itself is not touched; only the board's link to it goes."
          confirmLabel="Remove"
          onConfirm={() => remove(removing)}
          onCancel={() => setRemoving(null)}
        />
      )}
    </section>
  );
}

function WorkLinkRow({ link, onRemove }: { link: WorkLink; onRemove: () => void }) {
  const host = linkHost(link);
  const name = link.kind === "pr" ? `pull request ${urlPathText(link.url)}` : `commit ${shortSha(link.sha)}`;
  return (
    <li className="work-link">
      {link.kind === "pr" ? <GitPullRequest size={16} aria-hidden="true" /> : <GitCommitHorizontal size={16} aria-hidden="true" />}
      <span className="visually-hidden">{link.kind === "pr" ? "Pull request" : "Commit"}</span>
      <WorkLinkTarget link={link} />
      {host !== "" && <span className="host-chip">{host}</span>}
      <span className="work-link__meta muted small">
        {link.added_by.name} · <time dateTime={link.added_at}>{relativeTime(link.added_at)}</time>
      </span>
      <button type="button" className="icon-button icon-button--small" aria-label={`Remove ${name}`} onClick={onRemove}>
        <X size={16} aria-hidden="true" />
      </button>
    </li>
  );
}

function WorkLinkTarget({ link }: { link: WorkLink }) {
  const text = link.kind === "pr" ? urlPathText(link.url) : shortSha(link.sha);
  const className = link.kind === "commit" ? "work-link__target sha" : "work-link__target";
  if (link.url === "") {
    return <span className={className}>{text}</span>;
  }
  return (
    <a className={className} href={link.url} target="_blank" rel="noopener noreferrer" title={link.url}>
      {text}
      <ExternalLink size={12} aria-hidden="true" />
    </a>
  );
}
