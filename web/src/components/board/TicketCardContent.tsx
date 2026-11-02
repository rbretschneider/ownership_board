import { Bot, CircleAlert, GitPullRequest, Hand, Hourglass, MessageCircleQuestion, Zap } from "lucide-react";
import type { Repo, Stage, TicketSummary } from "../../api/types";
import { useLeaseClock } from "../../hooks/useLeaseClock";
import { cardActivity, hasLiveLease } from "../../lib/activity";
import { ticketRefs } from "../../lib/relations";
import { KindTag } from "../common/KindTag";
import { StageChip } from "../common/StageChip";
import { RepoChip } from "../repos/RepoChip";

/** The visual body of a card, shared by the card and its drag ghost. */
export function TicketCardContent({ ticket, stages, repo, now }: { ticket: TicketSummary; stages: Stage[]; repo: Repo | undefined; now?: number }) {
  const clock = useLeaseClock(now === undefined ? ticket.lease?.expires_at : undefined);
  const at = now ?? clock;
  const liveLease = hasLiveLease(ticket, at);
  const activity = cardActivity(ticket, at);
  const showActivity = ticket.stage !== "done" && (ticket.is_autonomous || ticket.lease !== null || ticket.lane !== "todo" || activity.blocked);
  const model = ticket.work_model?.split(" · ")[0];
  const isBlocked = ticket.blocked_reason !== "";
  const isWaiting = ticket.waiting_on.length > 0;
  return (
    <>
      <div className="card__top">
        <span className="card__id">#{ticket.id}</span>
        <KindTag kind={ticket.kind} />
        {showActivity && (
          <span className={`badge card__activity${activity.blocked ? " badge--blocked" : activity.working ? " badge--working" : ticket.is_autonomous ? " badge--autopilot" : ""}`}
            title={`${ticket.is_autonomous ? "On autopilot · " : ""}${activity.label === "Queued" ? "Waiting for a worker" : activity.label}${ticket.lease ? ` · ${model || ticket.lease.holder}` : ""}`}
            aria-label={`${ticket.is_autonomous ? "Autopilot: " : ""}${activity.label}`}>
            {ticket.is_autonomous && <Zap className="autopilot-spark" size={12} aria-hidden="true" />}
            {activity.working && <span className="pixel-loader" aria-hidden="true">{Array.from({ length: 9 }, (_, i) => <i key={i} />)}</span>}
            {!activity.working && ticket.lane === "todo" && ticket.is_autonomous && !activity.blocked && !ticket.needs_owner ? "Autopilot" : activity.label}
          </span>
        )}
      </div>
      <p className="card__title">{ticket.title}</p>
      {(ticket.stage !== "done" || ticket.needs_owner || isBlocked || isWaiting) && (
        <div className="card__status">
          {ticket.stage !== "done" && !activity.working && <StageChip stage={ticket.stage} stages={stages} />}
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
          {isWaiting && (
            <span className="badge badge--waiting" title="Agents cannot claim this until these tickets are merged">
              <Hourglass size={13} aria-hidden="true" />
              Waiting on {ticketRefs(ticket.waiting_on)}
            </span>
          )}
        </div>
      )}
      <div className="card__meta">
        <RepoChip repo={repo} />
        {liveLease && ticket.stage !== "done" && (
          <span className="badge card__model" title={`Working on this: ${ticket.lease?.holder}`}>
            <Bot size={12} aria-hidden="true" />{model || "Agent working"}
          </span>
        )}
        {ticket.open_questions > 0 && (
          <span className="badge badge--human">
            <MessageCircleQuestion size={13} aria-hidden="true" />
            {ticket.open_questions === 1 ? "1 question" : `${ticket.open_questions} questions`}
          </span>
        )}
        {ticket.pr_url !== "" && ticket.stage !== "done" && (
          <span className="badge" title={ticket.pr_url}>
            <GitPullRequest size={13} aria-hidden="true" />
            Delivery checks
          </span>
        )}
      </div>
    </>
  );
}
