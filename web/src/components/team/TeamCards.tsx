import type { TeamBuildHost, TeamManager, TeamRunner, TeamTicket } from "../../api/types";
import { formatElapsed } from "../../lib/buildClock";
import {
  clockTime, formatTokens, formatUSD, hostLoad, humanizeDoing, runnerEngine, runnerShortName, RUNNER_STATES, ticketClock, ticketRole, ticketWhere,
} from "../../lib/team";

const SUMMARY_LIMIT = 260;

/** Props for {@link ManagerCard}. */
export interface ManagerCardProps {
  manager: TeamManager;
  onOpen: () => void;
}

/** The board manager on top of the chart: latest decision, spend, flags. */
export function ManagerCard({ manager, onOpen }: ManagerCardProps) {
  const latest = manager.sessions[0];
  const summary = manager.latest_summary.length > SUMMARY_LIMIT ? `${manager.latest_summary.slice(0, SUMMARY_LIMIT - 1)}…` : manager.latest_summary;
  return (
    <button type="button" className="team-card team-manager" onClick={onOpen} aria-label="Board manager: open history">
      <span className="team-manager__head">
        <span className="team-row">
          <span className={`team-dot${manager.enabled ? " team-dot--live" : " team-dot--grey"}`} aria-hidden="true" />
          <span className="team-manager__name">Board manager</span>
          <span className="team-faint">{[latest?.model, manager.enabled ? "on" : "off"].filter(Boolean).join(" · ")}</span>
        </span>
        <span className="team-objectives">Cost › Progress › Speed</span>
      </span>
      <span className="team-manager__summary">
        {manager.latest_at === null
          ? "No manager sessions recorded yet."
          : <>Last review {clockTime(manager.latest_at)}. {summary}</>}
      </span>
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
      <span className="team-link-text">Tap for history log</span>
    </button>
  );
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
  const name = runnerShortName(runner.name);
  const ticket = runner.ticket_id === null ? "none" : `#${runner.ticket_id}`;
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
      aria-label={`${name}, ${state.label}, holds ${ticket}. Open details`}
    >
      <span className="team-row team-row--between">
        <span className="team-row">
          <span className={`team-dot team-dot--${state.tone}${runner.state === "working" ? " team-dot--live" : ""}`} aria-hidden="true" />
          <span className="team-runner__name">{name}</span>
        </span>
        <span className={`team-pill team-pill--${state.tone}`}>{state.label}</span>
      </span>
      <span className="team-faint">{runnerEngine(runner)}</span>
      <span className="team-runner__doing">{humanizeDoing(runner.doing)}</span>
      <span className="team-chips">
        <span className="team-chip team-chip--ticket">{ticket}</span>
        <span className="team-chip">{runner.profile_version === "" ? "profile not reported" : runner.profile_version}</span>
      </span>
    </button>
  );
}

/** Props for {@link TicketCard}. */
export interface TeamTicketCardProps {
  ticket: TeamTicket;
  hosts: TeamBuildHost[];
  isLinked: boolean;
  now: number;
}

/** A ticket in flight: role, where it is, its build clock and tokens. */
export function TeamTicketCard({ ticket, hosts, isLinked, now }: TeamTicketCardProps) {
  const role = ticketRole(ticket.stage);
  return (
    <article className={`team-card team-ticket${isLinked ? " team-card--linked" : ""}`} data-ticket={ticket.id}>
      <div className="team-row team-row--between">
        <a className="team-ticket__id" href={`#/t/${ticket.id}`}>#{ticket.id}</a>
        <span className={`team-pill team-pill--small team-pill--${role.tone}`}>{role.label}</span>
      </div>
      <div className="team-ticket__title">{ticket.title}</div>
      <div className="team-faint">{ticketWhere(ticket, hosts)}</div>
      <div className="team-ticket__foot">
        <span>{ticketClock(ticket, now)}</span>
        <span className="team-faint">{formatTokens(ticket.usage?.weighted_tokens)}</span>
      </div>
    </article>
  );
}

/** Props for {@link BuildHostCard}. */
export interface BuildHostCardProps {
  host: TeamBuildHost;
  now: number;
}

const OS_LABELS: Record<TeamBuildHost["os"], string> = { windows: "Windows", linux: "Linux", macos: "macOS" };

/** A build host: one box per slot with the job, phase and elapsed time, then its queue. */
export function BuildHostCard({ host, now }: BuildHostCardProps) {
  const load = hostLoad(host);
  const slots = Math.max(host.max_parallel, host.running.length);
  return (
    <article className="team-card team-host">
      <div className="team-row team-row--between">
        <h3 className="team-host__name">{host.name} <span className="team-faint">· {OS_LABELS[host.os]} build host</span></h3>
        <span className={`team-pill team-pill--${load.tone}`}>{load.text}</span>
      </div>
      {slots > 0 && (
        <div className="team-slots">
          {Array.from({ length: slots }, (_, i) => {
            const job = host.running[i];
            const started = job?.started_at ? Date.parse(job.started_at) : NaN;
            return (
              <div key={i} className={`team-slot${job ? " team-slot--busy" : ""}`}>
                <div className="team-faint">Slot {i + 1}</div>
                <div className="team-slot__job">{job ? `#${job.ticket_id} · ${job.phase || job.status}` : "free"}</div>
                <div className="team-slot__bar" aria-hidden="true"><div className="team-slot__fill" /></div>
                <div className="team-slot__clock">{job && Number.isFinite(started) ? formatElapsed(now - started) : "—"}</div>
              </div>
            );
          })}
        </div>
      )}
      <div className="team-host__queue">
        {host.queued.length === 0
          ? "Queue empty"
          : <>Queue: {host.queued.map((job) => `#${job.ticket_id} waiting ${formatElapsed(now - Date.parse(job.queued_at))}`).join(" · ")}</>}
      </div>
    </article>
  );
}
