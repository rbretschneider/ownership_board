import { Bot, CircleAlert, GitPullRequest, Hand, MessageCircleQuestion } from "lucide-react";
import type { Repo, Stage, TicketSummary } from "../../api/types";
import { KindTag } from "../common/KindTag";
import { StageChip } from "../common/StageChip";
import { RepoChip } from "../repos/RepoChip";

/** The visual body of a card, shared by the card and its drag ghost. */
export function TicketCardContent({ ticket, stages, repo }: { ticket: TicketSummary; stages: Stage[]; repo: Repo | undefined }) {
  const isBlocked = ticket.blocked_reason !== "";
  return (
    <>
      <div className="card__top">
        <span className="card__id">#{ticket.id}</span>
        <KindTag kind={ticket.kind} />
        {ticket.lease !== null && (
          <span className="badge badge--agent" title={`Claimed by ${ticket.lease.holder}`}>
            <span className="pulse" aria-hidden="true" />
            <Bot size={13} aria-hidden="true" />
            AI working
          </span>
        )}
      </div>
      <p className="card__title">{ticket.title}</p>
      {(ticket.stage !== "done" || ticket.needs_owner || isBlocked) && (
        <div className="card__status">
          {ticket.stage !== "done" && <StageChip stage={ticket.stage} stages={stages} />}
          {ticket.needs_owner && (
            <span className="badge badge--needs">
              <Hand size={13} aria-hidden="true" />
              Needs you
            </span>
          )}
          {isBlocked && (
            <span className="badge badge--blocked" title={ticket.blocked_reason}>
              <CircleAlert size={13} aria-hidden="true" />
              Blocked
            </span>
          )}
        </div>
      )}
      <div className="card__meta">
        <RepoChip repo={repo} />
        {ticket.open_questions > 0 && (
          <span className="badge badge--human">
            <MessageCircleQuestion size={13} aria-hidden="true" />
            {ticket.open_questions === 1 ? "1 question" : `${ticket.open_questions} questions`}
          </span>
        )}
        {ticket.pr_url !== "" && (
          <span className="badge" title={ticket.pr_url}>
            <GitPullRequest size={13} aria-hidden="true" />
            PR
          </span>
        )}
      </div>
    </>
  );
}
