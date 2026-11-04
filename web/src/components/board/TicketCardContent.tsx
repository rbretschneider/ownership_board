import { Bot, CircleAlert, Hand, Hourglass, MessageCircleQuestion, Server, Zap } from "lucide-react";
import type { Repo, Stage, TicketSummary } from "../../api/types";
import { useLeaseClock } from "../../hooks/useLeaseClock";
import { cardActivity, hasLiveLease } from "../../lib/activity";
import { ticketRefs } from "../../lib/relations";
import { parseWorkModel, shortTarget } from "../../lib/workModel";
import { KindTag } from "../common/KindTag";
import { StageChip } from "../common/StageChip";
import { RepoChip } from "../repos/RepoChip";
import { StageStepper } from "./StageStepper";

/** Three stacked chevrons, like rank stripes: the story earned a stronger model. */
function RankChevrons() {
  return (
    <svg className="rank-chevrons" width="11" height="12" viewBox="0 0 11 12" aria-hidden="true">
      <path d="M1.5 4.5 5.5 1.5 9.5 4.5M1.5 7.5 5.5 4.5 9.5 7.5M1.5 10.5 5.5 7.5 9.5 10.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

/**
 * The visual body of a card, shared by the card and its drag ghost.
 *
 * The top row answers "who is on it and what is happening" in one place:
 * id and kind on the left; the model and a single status pill (with the build
 * target when the work is on the build server) on the right. The meta row only
 * carries the repo and things that need the owner; the stepper shows progress.
 */
export function TicketCardContent({ ticket, stages, repo, now }: { ticket: TicketSummary; stages: Stage[]; repo: Repo | undefined; now?: number }) {
  const clock = useLeaseClock(now === undefined ? ticket.lease?.expires_at : undefined);
  const at = now ?? clock;
  const liveLease = hasLiveLease(ticket, at);
  const activity = cardActivity(ticket, at);
  const showActivity = ticket.stage !== "done" && (ticket.is_autonomous || ticket.lease !== null || ticket.lane !== "todo" || activity.blocked);
  const model = parseWorkModel(ticket.work_model);
  const showModel = ticket.stage !== "done" && (liveLease || (activity.buildServer === true && model !== undefined));
  const isBlocked = ticket.blocked_reason !== "";
  const isWaiting = ticket.waiting_on.length > 0;
  const phase = activity.buildServer ? activity.label.split(" · ")[0] : activity.label;
  const label = activity.buildServer ? phase
    : isWaiting && !isBlocked ? `Waiting on ${ticketRefs(ticket.waiting_on)}`
    : !activity.working && ticket.lane === "todo" && ticket.is_autonomous && !activity.blocked && !ticket.needs_owner ? "Autopilot"
    : activity.label;
  const statusTitle = activity.buildServer ? `Build server · ${phase} · ${activity.target}`
    : isBlocked ? `Blocked: ${ticket.blocked_reason}`
    : isWaiting ? "Agents cannot claim this until these tickets are merged"
    : `${ticket.is_autonomous ? "On autopilot · " : ""}${activity.label === "Queued" ? "Waiting for a worker" : activity.label}`;
  const escalated = model?.escalatedAfter !== undefined;
  return (
    <>
      <div className="card__top">
        <span className="card__id">#{ticket.id}</span>
        <KindTag kind={ticket.kind} />
        {(showModel || showActivity) && (
          <span className="card__top-right">
            {showModel && (
              <span className={`card__model${escalated ? " card__model--escalated" : ""}`}
                title={escalated ? `Escalated to ${model?.name} at high reasoning after ${model?.escalatedAfter} rejected code passes` : liveLease ? `Assigned model for ${ticket.lease?.holder}` : "Last reported model"}>
                {escalated ? <RankChevrons /> : <Bot size={12} aria-hidden="true" />}
                <span className="card__model-name">{model?.name ?? "Agent working"}</span>
              </span>
            )}
            {showActivity && (
              <span className={`badge card__activity${activity.blocked ? " badge--blocked" : activity.activeBuild ? " badge--build-server" : activity.buildServer ? " badge--build-target" : activity.working ? " badge--working" : ticket.is_autonomous ? " badge--autopilot" : ""}`}
                title={statusTitle} aria-label={`${ticket.is_autonomous ? "Autopilot: " : ""}${activity.buildServer ? `${phase} on ${activity.target}` : label}`}>
                {activity.buildServer ? <Server size={12} aria-hidden="true" />
                  : isBlocked ? <CircleAlert size={12} aria-hidden="true" />
                  : isWaiting ? <Hourglass size={12} aria-hidden="true" />
                  : ticket.is_autonomous && <Zap className="autopilot-spark" size={12} aria-hidden="true" />}
                {(activity.working || activity.activeBuild) && <span className="pixel-loader" aria-hidden="true">{Array.from({ length: 9 }, (_, i) => <i key={i} />)}</span>}
                <span className="card__activity-label">{label}</span>
                {activity.buildServer && activity.target && <span className="card__activity-target">{shortTarget(activity.target)}</span>}
              </span>
            )}
          </span>
        )}
      </div>
      <p className="card__title">{ticket.title}</p>
      <div className="card__meta">
        <RepoChip repo={repo} />
        {/* Without a status pill, the stage and owner signals still need a place. */}
        {!showActivity && ticket.stage !== "done" && <StageChip stage={ticket.stage} stages={stages} />}
        {!showActivity && ticket.needs_owner && (
          <span className="badge badge--needs">
            <Hand size={13} aria-hidden="true" />
            Needs you
          </span>
        )}
        {!showActivity && isBlocked && (
          <span className="badge badge--blocked" title={ticket.blocked_reason}>
            <CircleAlert size={13} aria-hidden="true" />
            Blocked
          </span>
        )}
        {!showActivity && isWaiting && (
          <span className="badge badge--waiting" title="Agents cannot claim this until these tickets are merged">
            <Hourglass size={13} aria-hidden="true" />
            Waiting on {ticketRefs(ticket.waiting_on)}
          </span>
        )}
        {ticket.open_questions > 0 && (
          <span className="badge badge--human">
            <MessageCircleQuestion size={13} aria-hidden="true" />
            {ticket.open_questions === 1 ? "1 question" : `${ticket.open_questions} questions`}
          </span>
        )}
      </div>
      <StageStepper ticket={ticket} />
    </>
  );
}
