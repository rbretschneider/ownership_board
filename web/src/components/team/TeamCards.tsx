import { Bot } from "lucide-react";
import type { KeyboardEvent, MouseEvent, PointerEvent, ReactNode } from "react";
import type { CIRunners, ManagerActivity, TeamBuildHost, TeamHostJob, TeamManager, TeamQueuedJob, TeamRunner, TeamTicket } from "../../api/types";
import { useNow } from "../../hooks/useNow";
import { useRunnerLabel, useRunnerNames } from "../../hooks/useRunnerNames";
import { formatElapsed } from "../../lib/buildClock";
import { nameRunnersInText } from "../../lib/runnerNames";
import { relativeTime } from "../../lib/relativeTime";
import {
  CI_SAMPLE_STALE_MS, ciRunnerLabels, ciRunnerStatus, clockTime, formatUSD, hostStatus, humanizeDoing, runnerModelChip, runnerPool,
  RUNNER_STATES, runnerFamily, slotStatus, ticketClock, type TicketActivity, type WorkStatus,
} from "../../lib/team";
import { MiningSprite } from "../board/MiningSprite";
import { StageStepper } from "../board/StageStepper";
import { RankChevrons, UsageBadge } from "../board/UsageBadge";
import { EngineSpinner } from "../common/EngineSpinner";

/** Props for {@link CIRunnersCard}. */
export interface CIRunnersCardProps {
  sample: CIRunners;
  now: number;
}

/**
 * The repos' GitHub Actions runners as one compact strip at the bottom: a
 * status dot and name each, with state, labels and repo in the tooltip.
 */
export function CIRunnersCard({ sample, now }: CIRunnersCardProps) {
  const observed = sample.observed_at === null ? NaN : Date.parse(sample.observed_at);
  const isStale = Number.isFinite(observed) && now - observed > CI_SAMPLE_STALE_MS;
  return (
    <section className="team-ci" aria-label="CI runners">
      <h2 className="team-heading team-ci__heading">CI runners</h2>
      {sample.runners.length === 0
        ? <span className="team-faint">none registered</span>
        : (
          <ul className="team-ci__list">
            {sample.runners.map((runner) => {
              const state = ciRunnerStatus(runner);
              const details = [runner.name, state.text, ciRunnerLabels(runner), runner.repo].filter((part) => part !== "").join(" · ");
              return (
                <li key={`${runner.repo}/${runner.name}`} className="team-ci__item" data-ci-runner={runner.name} title={details} aria-label={details}>
                  <StatusDot status={state} />
                  <span className="team-ci__name" aria-hidden="true">{runner.name}</span>
                </li>
              );
            })}
          </ul>
        )}
      {isStale && sample.observed_at !== null && (
        <span className="team-pill team-pill--amber team-pill--small">sampled {relativeTime(sample.observed_at, new Date(now))}</span>
      )}
    </section>
  );
}

/** Props for {@link ManagerCard}. */
export interface ManagerCardProps {
  manager: TeamManager;
  onOpen: () => void;
}

/** The board manager on top of the chart: latest decision, spend, flags. */
export function ManagerCard({ manager, onOpen }: ManagerCardProps) {
  const latest = manager.sessions[0];
  const { directory } = useRunnerNames();
  // Older servers send no headline: the summary's first line stands in.
  const rawHeadline = manager.latest_headline ?? manager.latest_summary.split("\n").find((line) => line.trim() !== "")?.trim() ?? "";
  const headline = nameRunnersInText(rawHeadline, directory);
  const activity = manager.activity;
  const isWorking = activity?.state === "working";
  return (
    <button type="button" className={`team-card team-manager${isWorking ? " team-manager--working" : ""}`} data-manager="" onClick={onOpen} aria-label="Board manager: open history">
      <span className="team-manager__head">
        <span className="team-row">
          <span className={`team-dot ${isWorking ? "team-dot--busy team-dot--live" : "team-dot--idle"}`} aria-hidden="true" />
          <span className="team-manager__name">Board manager</span>
          <span className="team-faint">{[latest?.model, manager.enabled ? "on" : "off"].filter(Boolean).join(" · ")}</span>
        </span>
        <span className="team-objectives">Cost › Progress › Speed</span>
      </span>
      <span className="team-manager__headline">
        {manager.latest_at === null
          ? "No manager sessions recorded yet."
          : <><span className="team-manager__when">{clockTime(manager.latest_at)}</span> {headline}</>}
      </span>
      {isWorking && activity && (
        <span className="team-manager__now" role="status"><EngineSpinner size={28} />{managerNow(activity)}</span>
      )}
      <span className="team-stats">
        <span className="team-stat">
          <span className="team-stat__label">Spend today</span>
          <span className="team-stat__value">{formatUSD(manager.today.cost_usd)} <span className="team-stat__of">{manager.budget_off ? "no daily cap" : `of $${manager.daily_budget_usd}`}</span></span>
        </span>
        <span className="team-stat">
          <span className="team-stat__label">Open flags</span>
          <span className={`team-stat__value${manager.open_flags > 0 ? " team-tone--amber" : ""}`}>{manager.open_flags}</span>
        </span>
        <span className="team-stat">
          <span className="team-stat__label">Sessions today</span>
          <span className="team-stat__value team-tone--green">{manager.today.sessions}</span>
        </span>
      </span>
      <span className="team-hint">Tap for history log</span>
    </button>
  );
}

/** "Working on #64 and #55 · builder-1" or "Reviewing the board", for the card while a session runs. */
function managerNow(activity: ManagerActivity): string {
  const verb = activity.kind === "review" ? "Reviewing the board" : activity.kind === "repair" ? "Repairing" : "Working on";
  const targets = [...activity.ticket_ids.map((id) => `#${id}`), ...(activity.host ? [activity.host] : [])];
  if (activity.kind === "review") return targets.length > 0 ? `${verb}: ${targets.join(", ")}` : verb;
  return targets.length > 0 ? `${verb} ${targets.join(", ")}` : `${verb} a deployment flag`;
}

/**
 * The runner's little worker, as on the Board's cards: a Claude crab or a
 * Codex terminal that swings its pickaxe while the runner works and stands
 * still, dimmed, while it waits or idles. A runner of no known family keeps a dot.
 */
function RunnerFigure({ runner }: { runner: TeamRunner }) {
  const family = runnerFamily(runner.name);
  const state = RUNNER_STATES[runner.state];
  if (family === undefined) {
    return <span className={`team-dot team-dot--${state.tone}${runner.state === "working" ? " team-dot--live" : ""}`} aria-hidden="true" />;
  }
  return (
    <span className={`team-runner__figure team-runner__figure--${runner.state}`} data-figure={family}>
      <MiningSprite family={family} />
    </span>
  );
}

/** Props for {@link RunnerCard}. */
export interface RunnerCardProps {
  runner: TeamRunner;
  isActive: boolean;
  onEnter: () => void;
  onLeave: () => void;
  onOpen: () => void;
  /** Set for an idle runner: its countdown to the next claim and what blocks it. */
  poll?: IdlePoll;
}

/** One runner: state, engine, what it is doing, the ticket it holds. */
/** An idle runner's claim cadence: seconds between claims, and why its claims come back empty ("" when nothing blocks them). */
export interface IdlePoll {
  seconds: number;
  blocked: string;
}

const RING_R = 7;
const RING_C = 2 * Math.PI * RING_R;

/**
 * A stopwatch ring counting down to the idle runner's next claim, from its
 * last call to the board. Past zero it reads "checking…" until the next team
 * refresh brings a newer last call and the ring starts over.
 */
export function PollCountdown({ lastSeen, seconds }: { lastSeen: string | null; seconds: number }) {
  const now = useNow(1000);
  if (lastSeen === null) return null;
  const left = Math.ceil(seconds - (now - Date.parse(lastSeen)) / 1000);
  const fraction = Math.min(1, Math.max(0, left / seconds));
  const label = left > 0 ? `next check in ${left}s` : "checking…";
  return (
    <span className="team-poll" role="timer" aria-label={`Idle: ${label}`}>
      <svg className="team-poll__ring" width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
        <circle cx="9" cy="9" r={RING_R} className="team-poll__track" />
        <circle cx="9" cy="9" r={RING_R} className="team-poll__arc"
          strokeDasharray={RING_C} strokeDashoffset={RING_C * (1 - fraction)} transform="rotate(-90 9 9)" />
      </svg>
      <span>{label}</span>
    </span>
  );
}

export function RunnerCard({ runner, isActive, onEnter, onLeave, onOpen, poll }: RunnerCardProps) {
  const state = RUNNER_STATES[runner.state];
  const name = useRunnerLabel(runner.name, runner);
  const ticket = runner.ticket_id === null ? "none" : `#${runner.ticket_id}`;
  // The model is what it last reported for a ticket. Without a ticket that is
  // history, not what it runs now, so a free runner shows only its pool.
  const current = runner.ticket_id === null ? { ...runner, model: "" } : runner;
  const chip = runnerModelChip(current);
  return (
    <button
      type="button"
      className={`team-card team-runner${isActive ? " team-card--active" : ""}`}
      data-runner={runner.name}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      onFocus={onEnter}
      onBlur={onLeave}
      onClick={onOpen}
      aria-label={`${name.detail}, ${state.label}, holds ${ticket}. Open details`}
    >
      <span className="team-row team-row--between">
        <span className="team-row">
          <RunnerFigure runner={runner} />
          <span className="team-runner__name" title={name.title}>{name.detail}{name.badge !== "" && <span className="runner-name__family">{name.badge}</span>}</span>
        </span>
        <span className={`team-pill team-pill--${state.tone}`}>{state.label}</span>
      </span>
      <span className="team-faint">{runnerPool(current)}</span>
      <span className="team-runner__doing">{humanizeDoing(runner.doing)}</span>
      {poll !== undefined && <PollCountdown lastSeen={runner.last_seen} seconds={poll.seconds} />}
      {poll !== undefined && poll.blocked !== "" && <span className="team-poll__blocked">{poll.blocked}</span>}
      {(chip !== undefined || runner.profile_version !== "") && (
        <span className="team-chips">
          {chip !== undefined && (
            <span className={`card__model${chip.escalatedAfter !== undefined ? " card__model--escalated" : ""}`}
              title={chip.escalatedAfter !== undefined ? `Escalated to ${chip.name} at high reasoning after ${chip.escalatedAfter} rejected code passes` : "Reported model"}>
              {chip.escalatedAfter !== undefined ? <RankChevrons /> : <Bot size={12} aria-hidden="true" />}
              <span className="card__model-name">{chip.name}</span>
            </span>
          )}
          {runner.profile_version !== "" && <span className="team-chip">{runner.profile_version}</span>}
        </span>
      )}
    </button>
  );
}

/** Props for {@link TeamTicketCard}. */
export interface TeamTicketCardProps {
  ticket: TeamTicket;
  activity: TicketActivity;
  isLinked: boolean;
  now: number;
  /** Set when the ticket builds in a host slot: hovering, focusing or tapping the card shows that slot. */
  onPairHover?: (ticketId: number | null) => void;
  onPairToggle?: (ticketId: number) => void;
}

/**
 * A ticket being worked, nested under the runner or build-host slot that has
 * it: what it is doing, then the Board card's own token badge and stage
 * stepper with attempt counts, the signals that show a problem at a glance.
 */
export function TeamTicketCard({ ticket, activity, isLinked, now, onPairHover, onPairToggle }: TeamTicketCardProps) {
  const clock = ticketClock(ticket, now);
  const pairProps = onPairToggle === undefined ? {} : {
    "data-pairs-slot": true,
    onPointerEnter: (e: PointerEvent<HTMLElement>) => { if (e.pointerType === "mouse") onPairHover?.(ticket.id); },
    onPointerLeave: (e: PointerEvent<HTMLElement>) => { if (e.pointerType === "mouse") onPairHover?.(null); },
    onFocus: () => onPairHover?.(ticket.id),
    onBlur: () => onPairHover?.(null),
    // A tap anywhere but the ticket link pins the pairing, so it reads on a phone.
    onClick: (e: MouseEvent<HTMLElement>) => { if (!(e.target instanceof Element && e.target.closest("a"))) onPairToggle(ticket.id); },
  };
  return (
    <article className={`team-card team-ticket team-ticket--${activity.group}${isLinked ? " team-card--linked" : ""}`} data-ticket={ticket.id} {...pairProps}>
      <div className="team-row team-row--between team-row--top">
        <a className="team-ticket__id" href={`#/t/${ticket.id}`}>#{ticket.id}</a>
        <UsageBadge usage={ticket.usage} />
      </div>
      <div className="team-ticket__title">{ticket.title}</div>
      <div className={`team-ticket__activity team-tone--${activity.tone}`}>
        {activity.text}{clock !== "—" ? <span className="team-faint"> · {clock}</span> : null}
      </div>
      <StageStepper ticket={ticket} />
    </article>
  );
}

/** A runner and the ticket it holds, as one branch of the org chart. */
export function TeamBranch({ children }: { children: ReactNode }) {
  return <div className="team-branch">{children}</div>;
}

/** A status dot on the shared idle / busy / problem scale. */
export function StatusDot({ status }: { status: WorkStatus }) {
  return <span className={`team-dot team-dot--${status.tone}`} aria-hidden="true" />;
}

/** A busy slot's working mark in place of its dot: the engine spinner. */
function BuildMatrix() {
  return <span className="team-slot__matrix"><EngineSpinner size={22} /></span>;
}

/** Props for {@link BuildHostCard}. */
export interface BuildHostCardProps {
  host: TeamBuildHost;
  now: number;
  /** Renders the ticket card for a slot whose ticket no runner holds; null otherwise. */
  renderSlotTicket?: (ticketId: number) => ReactNode;
  /** The runner whose ticket a slot builds, as a short label; "" when no runner holds it. */
  slotOwner?: (ticketId: number) => string;
  /** The ticket whose card and slot are shown as a pair, if any; pinned is the one a tap fixed. */
  pairedTicket?: number | null;
  pinnedTicket?: number | null;
  /** Hovering or focusing a paired slot shows its pair; null ends it. */
  onPairHover?: (ticketId: number | null) => void;
  /** Tapping a paired slot pins or unpins its pair, so it reads on a phone too. */
  onPairToggle?: (ticketId: number) => void;
  /** Cancels a running or queued job; given only to administrators, who alone may cancel. */
  onCancel?: (job: TeamHostJob | TeamQueuedJob) => void;
  /** Where a cancel the owner started stands, by job id; undefined when none is under way. */
  cancelState?: (jobId: string) => CancelState | undefined;
  /** Tickets the page already shows as cards under "Waiting for a build slot"; the queue line leaves them out so nothing shows twice. */
  shownElsewhere?: ReadonlySet<number>;
}

/** A cancel the owner started: the request is in flight, or it went through. */
export type CancelState = "cancelling" | "cancelled";

/**
 * A small Cancel control that never toggles the slot it sits in. While the
 * request runs it shows the engine spinner and "Cancelling…"; once it went
 * through it reads "Cancelled" until the next refresh drops the job.
 */
export function CancelJobButton({ job, label, onCancel, state, text = "Cancel" }: {
  job: TeamHostJob | TeamQueuedJob;
  label: string;
  onCancel: (job: TeamHostJob | TeamQueuedJob) => void;
  state?: CancelState;
  text?: string;
}) {
  if (state === "cancelled") {
    return <span className="team-cancel team-cancel--done" role="status">Cancelled</span>;
  }
  if (state === "cancelling") {
    return (
      <button type="button" className="team-cancel team-cancel--busy" disabled aria-busy="true" aria-label={`Cancelling the run for #${job.ticket_id}`}>
        <EngineSpinner size={12} /> Cancelling…
      </button>
    );
  }
  return (
    <button type="button" className="team-cancel" aria-label={label} title={label}
      onClick={(e: MouseEvent<HTMLButtonElement>) => { e.stopPropagation(); onCancel(job); }}
      onKeyDown={(e: KeyboardEvent<HTMLButtonElement>) => e.stopPropagation()}>
      {text}
    </button>
  );
}

const OS_LABELS: Record<TeamBuildHost["os"], string> = { windows: "Windows", linux: "Linux", macos: "macOS" };

/**
 * One build server: a status dot and the build count in its header, then all
 * of its slots side by side, each with a bar "Slot 1 #175 · building · 7m" and
 * its Cancel. A slot whose ticket a runner holds pairs with that ticket's card
 * under the runner; a ticket no runner holds is drawn inside its slot, under
 * the bar. The queue line shows only runs no waiting card already shows.
 */
export function BuildHostCard({ host, now, renderSlotTicket, slotOwner, pairedTicket = null, pinnedTicket = null, onPairHover, onPairToggle, onCancel, cancelState, shownElsewhere }: BuildHostCardProps) {
  // A queued run the owner cancelled leaves the list at once, before the refresh
  // lands; one whose ticket already has its own waiting card is not repeated here.
  const queued = host.queued.filter((job) => (job.id === undefined || cancelState?.(job.id) !== "cancelled")
    && !(shownElsewhere?.has(job.ticket_id) ?? false));
  const status = hostStatus(host, now);
  const slots = Math.max(host.max_parallel, host.running.length);
  return (
    <article className={`team-card team-host team-host--${status.tone}`} data-host={host.name}
      style={{ flexGrow: Math.max(1, slots), flexBasis: `${Math.max(300, slots * 170)}px` }}>
      <div className="team-row team-row--between">
        <h3 className="team-host__name team-row">
          <StatusDot status={status} />
          <span>{host.name} <span className="team-faint">· {OS_LABELS[host.os]}</span></span>
        </h3>
        <span className={`team-status team-status--${status.tone}`}>{status.text}</span>
      </div>
      {slots > 0 && (
        <div className="team-slots" style={{ gridTemplateColumns: `repeat(${slots}, minmax(0, 1fr))` }}>
          {Array.from({ length: slots }, (_, i) => {
            const job = host.running[i];
            const slot = slotStatus(host, job, now);
            const owner = job === undefined ? "" : slotOwner?.(job.ticket_id) ?? "";
            const isPairable = job !== undefined && owner !== "" && onPairToggle !== undefined;
            const isPaired = job !== undefined && job.ticket_id === pairedTicket;
            const pairProps = isPairable ? {
              role: "button",
              tabIndex: 0,
              "aria-pressed": job.ticket_id === pinnedTicket,
              "aria-label": `Slot ${i + 1}: ${slot.text}, ${owner}'s ticket. Show its card`,
              "data-slot-ticket": job.ticket_id,
              onPointerEnter: (e: PointerEvent<HTMLDivElement>) => { if (e.pointerType === "mouse") onPairHover?.(job.ticket_id); },
              onPointerLeave: (e: PointerEvent<HTMLDivElement>) => { if (e.pointerType === "mouse") onPairHover?.(null); },
              onFocus: () => onPairHover?.(job.ticket_id),
              onBlur: () => onPairHover?.(null),
              onClick: () => onPairToggle?.(job.ticket_id),
              onKeyDown: (e: KeyboardEvent<HTMLDivElement>) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onPairToggle?.(job.ticket_id);
                }
              },
            } : {};
            // A ticket no runner holds is drawn inside its slot, under the slot's
            // status bar: one card per ticket, with the slot and Cancel on top.
            const card = job === undefined ? null : renderSlotTicket?.(job.ticket_id) ?? null;
            return (
              <div key={i} className={`team-slot team-slot--${slot.tone}${isPaired ? " team-card--linked" : ""}${isPairable ? " team-slot--pairable" : ""}`}
                data-slot={i + 1} {...pairProps}>
                <div className="team-row team-row--between team-slot__head">
                  <span className="team-row team-slot__label">
                    {slot.tone === "busy" ? <BuildMatrix /> : <StatusDot status={slot} />}
                    <span>Slot {i + 1}</span>
                    <span className="team-slot__job">{slot.text}</span>
                  </span>
                  <span className="team-row team-slot__actions">
                    {owner !== "" && <span className="team-faint team-slot__owner">{owner}</span>}
                    {job?.id && onCancel !== undefined && (
                      <CancelJobButton job={job} label={`Cancel the run for #${job.ticket_id} in slot ${i + 1}`} onCancel={onCancel} state={cancelState?.(job.id)} />
                    )}
                  </span>
                </div>
                {card}
              </div>
            );
          })}
        </div>
      )}
      {queued.length > 0 && onCancel === undefined && (
        <div className="team-host__queue">
          Queue: {queued.map((job) => `#${job.ticket_id} waiting ${formatElapsed(now - Date.parse(job.queued_at))}`).join(" · ")}
        </div>
      )}
      {/* Administrators see the queue as a list, each waiting run with its own Cancel. */}
      {queued.length > 0 && onCancel !== undefined && (
        <ul className="team-host__queue team-host__queue--list" aria-label={`${host.name} queue`}>
          {queued.map((job) => (
            <li key={job.id ?? job.ticket_id} className="team-row team-host__queue-item">
              <span>#{job.ticket_id} waiting {formatElapsed(now - Date.parse(job.queued_at))}{job.sha ? <span className="team-faint"> · {job.sha.slice(0, 8)}</span> : null}</span>
              {job.id && <CancelJobButton job={job} label={`Cancel the queued run for #${job.ticket_id}`} onCancel={onCancel} state={cancelState?.(job.id)} />}
            </li>
          ))}
        </ul>
      )}
    </article>
  );
}
