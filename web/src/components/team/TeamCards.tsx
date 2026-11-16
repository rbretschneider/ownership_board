import { Bot } from "lucide-react";
import type { KeyboardEvent, MouseEvent, PointerEvent, ReactNode } from "react";
import type { CIRunners, ManagerActivity, TeamBuildHost, TeamManager, TeamRunner, TeamTicket } from "../../api/types";
import { useRunnerLabel, useRunnerNames } from "../../hooks/useRunnerNames";
import { formatElapsed } from "../../lib/buildClock";
import { nameRunnersInText } from "../../lib/runnerNames";
import { relativeTime } from "../../lib/relativeTime";
import {
  CI_SAMPLE_STALE_MS, ciRunnerLabels, ciRunnerStatus, clockTime, formatUSD, hostStatus, humanizeDoing, runnerModelChip, runnerPool,
  RUNNER_STATES, slotStatus, ticketClock, type TicketActivity, type WorkStatus,
} from "../../lib/team";
import { StageStepper } from "../board/StageStepper";
import { RankChevrons, UsageBadge } from "../board/UsageBadge";

/** Props for {@link CIRunnersCard}. */
export interface CIRunnersCardProps {
  sample: CIRunners;
  now: number;
}

/** The repos' GitHub Actions runners: online and busy, idle, or offline. */
export function CIRunnersCard({ sample, now }: CIRunnersCardProps) {
  const observed = sample.observed_at === null ? NaN : Date.parse(sample.observed_at);
  const isStale = Number.isFinite(observed) && now - observed > CI_SAMPLE_STALE_MS;
  const repos = new Set(sample.runners.map((r) => r.repo));
  return (
    <article className="team-card team-host team-ci" aria-label="CI runners">
      <div className="team-row team-row--between">
        <h3 className="team-host__name">CI runners</h3>
        {isStale && sample.observed_at !== null && (
          <span className="team-pill team-pill--amber">sampled {relativeTime(sample.observed_at, new Date(now))}</span>
        )}
      </div>
      {sample.runners.length === 0
        ? <p className="team-empty">No runners registered.</p>
        : (
          <ul className="team-ci__list">
            {sample.runners.map((runner) => {
              const state = ciRunnerStatus(runner);
              const labels = ciRunnerLabels(runner);
              return (
                <li key={`${runner.repo}/${runner.name}`} className="team-ci__row" data-ci-runner={runner.name}>
                  <span className="team-row team-ci__who">
                    <StatusDot status={state} />
                    <span className="team-ci__name">{runner.name}</span>
                    {repos.size > 1 && <span className="team-faint">{runner.repo}</span>}
                  </span>
                  <span className="team-faint team-ci__state">{[state.text, labels].filter((part) => part !== "").join(" · ")}</span>
                </li>
              );
            })}
          </ul>
        )}
    </article>
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
        <span className="team-manager__now" role="status">{managerNow(activity)}</span>
      )}
      <span className="team-stats">
        <span className="team-stat">
          <span className="team-stat__label">Spend today</span>
          <span className="team-stat__value">{formatUSD(manager.today.cost_usd)} <span className="team-stat__of">of ${manager.daily_budget_usd}</span></span>
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

/** Props for {@link RunnerCard}. */
export interface RunnerCardProps {
  runner: TeamRunner;
  isActive: boolean;
  onEnter: () => void;
  onLeave: () => void;
  onOpen: () => void;
}

/** One runner: state, engine, what it is doing, the ticket it holds. */
export function RunnerCard({ runner, isActive, onEnter, onLeave, onOpen }: RunnerCardProps) {
  const state = RUNNER_STATES[runner.state];
  const name = useRunnerLabel(runner.name, runner);
  const ticket = runner.ticket_id === null ? "none" : `#${runner.ticket_id}`;
  const chip = runnerModelChip(runner);
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
          <span className={`team-dot team-dot--${state.tone}${runner.state === "working" ? " team-dot--live" : ""}`} aria-hidden="true" />
          <span className="team-runner__name" title={name.title}>{name.detail}{name.badge !== "" && <span className="runner-name__family">{name.badge}</span>}</span>
        </span>
        <span className={`team-pill team-pill--${state.tone}`}>{state.label}</span>
      </span>
      <span className="team-faint">{runnerPool(runner)}</span>
      <span className="team-runner__doing">{humanizeDoing(runner.doing)}</span>
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
}

const OS_LABELS: Record<TeamBuildHost["os"], string> = { windows: "Windows", linux: "Linux", macos: "macOS" };

/**
 * One build server: a status dot and the build count in its header, then all
 * of its slots side by side, each with a dot and "#175 · building · 7m". A
 * slot whose ticket a runner holds pairs with that ticket's card; a ticket
 * no runner holds shows under the row. The queue line shows only when jobs wait.
 */
export function BuildHostCard({ host, now, renderSlotTicket, slotOwner, pairedTicket = null, pinnedTicket = null, onPairHover, onPairToggle }: BuildHostCardProps) {
  const status = hostStatus(host, now);
  const slots = Math.max(host.max_parallel, host.running.length);
  const unheld = host.running.map((job) => renderSlotTicket?.(job.ticket_id) ?? null).filter((node) => node !== null);
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
            return (
              <div key={i} className={`team-slot team-slot--${slot.tone}${isPaired ? " team-card--linked" : ""}${isPairable ? " team-slot--pairable" : ""}`}
                data-slot={i + 1} {...pairProps}>
                <div className="team-row team-row--between">
                  <span className="team-row team-slot__label"><StatusDot status={slot} />Slot {i + 1}</span>
                  {owner !== "" && <span className="team-faint team-slot__owner">{owner}</span>}
                </div>
                <div className="team-slot__job">{slot.text}</div>
              </div>
            );
          })}
        </div>
      )}
      {/* Tickets no runner holds sit under the slot row at full width, so a narrow slot stays readable. */}
      {unheld.length > 0 && <div className="team-host__tickets">{unheld}</div>}
      {host.queued.length > 0 && (
        <div className="team-host__queue">
          Queue: {host.queued.map((job) => `#${job.ticket_id} waiting ${formatElapsed(now - Date.parse(job.queued_at))}`).join(" · ")}
        </div>
      )}
    </article>
  );
}
