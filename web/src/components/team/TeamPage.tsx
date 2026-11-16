import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Team, TeamRunner, TicketDetail } from "../../api/types";
import { useDebouncedCallback } from "../../hooks/useDebouncedCallback";
import { useEventStream } from "../../hooks/useEventStream";
import { useTeam } from "../../hooks/useTeam";
import { REFETCH_DEBOUNCE_MS } from "../../lib/constants";
import { relativeTime } from "../../lib/relativeTime";
import {
  activityLines, formatTokens, formatUSD, humanizeDoing, linkPath, runnerEngine, runnerShortName, RUNNER_STATES, type LinkLine,
} from "../../lib/team";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { PageShell } from "../common/PageShell";
import { BuildHostCard, CIRunnersCard, ManagerCard, RunnerCard, TeamTicketCard } from "./TeamCards";
import { TeamPanel, type PanelContent } from "./TeamPanel";

const CLOCK_TICK_MS = 30_000;

type Selection = { kind: "manager" } | { kind: "runner"; name: string };

/**
 * The Team view (#/team): the board as an org chart. The manager on top, its
 * runners, the tickets in flight and the build hosts. Hovering, focusing or
 * tapping a runner draws a line to the ticket it holds; tapping opens details.
 */
export function TeamPage() {
  const api = useApi();
  const { team, error, isLoading, refetch } = useTeam();
  const debouncedRefetch = useDebouncedCallback(refetch, REFETCH_DEBOUNCE_MS);
  useEventStream(debouncedRefetch, refetch);
  const now = useNow(CLOCK_TICK_MS);
  const containerRef = useRef<HTMLDivElement>(null);
  const [hovered, setHovered] = useState<string | null>(null);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [detail, setDetail] = useState<{ ticketId: number; value: TicketDetail | null; failed: boolean } | null>(null);
  const [line, setLine] = useState<(LinkLine & { key: string }) | null>(null);

  const selectedRunner = selection?.kind === "runner" ? team?.runners.find((r) => r.name === selection.name) ?? null : null;
  // The open panel keeps its runner's line; otherwise the hovered runner's.
  const linkedName = selectedRunner?.name ?? hovered;
  const linkedRunner = team?.runners.find((r) => r.name === linkedName) ?? null;
  const linkedTicket = linkedRunner?.ticket_id ?? null;

  const measure = useCallback(() => {
    const container = containerRef.current;
    if (container === null || linkedRunner === null || linkedTicket === null) {
      setLine(null);
      return;
    }
    const from = container.querySelector(`[data-runner="${cssEscape(linkedRunner.name)}"]`);
    const to = container.querySelector(`[data-ticket="${linkedTicket}"]`);
    if (from === null || to === null) {
      setLine(null);
      return;
    }
    const box = container.getBoundingClientRect();
    const scale = container.offsetWidth > 0 ? box.width / container.offsetWidth : 1;
    setLine({ key: `${linkedRunner.name}:${linkedTicket}`, ...linkPath(box, from.getBoundingClientRect(), to.getBoundingClientRect(), scale) });
  }, [linkedRunner, linkedTicket]);

  useLayoutEffect(() => {
    measure();
  }, [measure, team]);

  useEffect(() => {
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [measure]);

  // The runner panel's activity comes from its ticket's timeline.
  const detailTicket = selectedRunner?.ticket_id ?? null;
  useEffect(() => {
    if (detailTicket === null) {
      setDetail(null);
      return;
    }
    let isCurrent = true;
    setDetail((current) => (current?.ticketId === detailTicket ? current : { ticketId: detailTicket, value: null, failed: false }));
    api.ticket(detailTicket).then(
      (value) => { if (isCurrent) setDetail({ ticketId: detailTicket, value, failed: false }); },
      () => { if (isCurrent) setDetail((current) => ({ ticketId: detailTicket, value: current?.value ?? null, failed: true })); },
    );
    return () => { isCurrent = false; };
  }, [api, detailTicket, team]);

  const close = useCallback(() => {
    setSelection(null);
    setHovered(null);
  }, []);

  // Older servers send no CI sample, and a board without the sweep has none.
  const hasCISample = (team?.ci_runners?.observed_at ?? null) !== null;

  const panel = useMemo<PanelContent | null>(() => {
    if (team === null || selection === null) return null;
    if (selection.kind === "manager") return managerPanel(team);
    return selectedRunner === null ? null : runnerPanel(selectedRunner, detail?.ticketId === selectedRunner.ticket_id ? detail : null, now);
  }, [team, selection, selectedRunner, detail, now]);

  return (
    <PageShell page="team" title="Team" isWide>
      {team === null && isLoading && <LoadingState label="Loading team" />}
      {team === null && !isLoading && error !== null && (
        <ErrorState message={`Couldn't load the team: ${error}`} onRetry={refetch} />
      )}
      {team !== null && error !== null && (
        <p className="banner banner--error" role="alert">
          Couldn't refresh: {error}{" "}
          <button type="button" className="link-button" onClick={refetch}>Retry</button>
        </p>
      )}
      {team !== null && (
        <div className="team" ref={containerRef}>
          {line !== null && (
            <svg className="team-wire" aria-hidden="true" key={line.key}>
              <path className="team-wire__path" d={line.d} />
              <circle className="team-wire__end" cx={line.x2} cy={line.y2} r="4" />
            </svg>
          )}
          <p className="team-intro">Hover or tap a runner to see its ticket; tap for details.</p>
          <section className="team-top" aria-label="Manager">
            <ManagerCard manager={team.manager} onOpen={() => setSelection({ kind: "manager" })} />
          </section>
          <div className="team-feed" aria-hidden="true">
            <div className="team-feed__drop" />
            <div className="team-feed__rail" />
          </div>
          <section className="team-runners" aria-label="Runners">
            {team.runners.length === 0 && <p className="team-empty">No runners seen in the last few hours.</p>}
            {team.runners.map((runner) => (
              <RunnerCard
                key={runner.name}
                runner={runner}
                isActive={runner.name === linkedName}
                onEnter={() => setHovered(runner.name)}
                onLeave={() => setHovered((current) => (current === runner.name ? null : current))}
                onOpen={() => setSelection({ kind: "runner", name: runner.name })}
              />
            ))}
          </section>
          <section className="team-section" aria-label="Tickets in flight">
            <h2 className="team-heading">Tickets in flight</h2>
            {team.tickets.length === 0
              ? <p className="team-empty">Nothing in flight.</p>
              : (
                <div className="team-tickets">
                  {team.tickets.map((ticket) => (
                    <TeamTicketCard key={ticket.id} ticket={ticket} hosts={team.build_hosts} isLinked={ticket.id === linkedTicket} now={now} />
                  ))}
                </div>
              )}
          </section>
          <section className="team-section" aria-label="Build hosts and CI runners">
            <h2 className="team-heading">Build hosts and CI runners</h2>
            {team.build_hosts.length === 0 && !hasCISample && <p className="team-empty">No approved build hosts.</p>}
            {(team.build_hosts.length > 0 || hasCISample) && (
              <div className="team-hosts">
                {team.build_hosts.map((host) => <BuildHostCard key={host.id} host={host} now={now} />)}
                {hasCISample && team.ci_runners !== undefined && <CIRunnersCard sample={team.ci_runners} now={now} />}
              </div>
            )}
          </section>
          {panel !== null && <TeamPanel content={panel} onClose={close} />}
        </div>
      )}
    </PageShell>
  );
}

function managerPanel(team: Team): PanelContent {
  const m = team.manager;
  return {
    kind: "Manager",
    title: "Board manager",
    summary: "Team lead for autonomous work. Objectives in order: cost, progress, speed.",
    facts: [
      { label: "Spend today", value: `${formatUSD(m.today.cost_usd)} of $${m.daily_budget_usd}` },
      { label: "Remaining", value: formatUSD(Math.max(0, m.remaining_usd)) },
      { label: "Sessions today", value: String(m.today.sessions) },
      { label: "Open flags", value: String(m.open_flags) },
    ],
    logTitle: "History log",
    log: m.sessions.map((s) => ({ at: s.at, text: s.summary || "No summary recorded", tag: [formatUSD(s.cost_usd), s.model].filter(Boolean).join(" · "), tone: "accent" as const })),
    logNote: m.sessions.length === 0 ? "No manager sessions recorded yet." : undefined,
  };
}

function runnerPanel(runner: TeamRunner, detail: { value: TicketDetail | null; failed: boolean } | null, now: number): PanelContent {
  const ticket = detail?.value ?? null;
  const facts = [
    { label: "Model", value: runnerEngine(runner) },
    { label: "State", value: RUNNER_STATES[runner.state].label },
    { label: "Ticket", value: runner.ticket_id === null ? "none" : `#${runner.ticket_id}` },
    { label: "Last report", value: runner.last_seen === null ? "—" : relativeTime(runner.last_seen, new Date(now)) },
    { label: "Profile", value: runner.profile_version || "not reported" },
  ];
  if (ticket !== null) facts.push({ label: "Ticket tokens", value: formatTokens(ticket.usage?.weighted_tokens) });
  let logNote: string | undefined;
  if (runner.ticket_id === null) logNote = "No ticket held, so no recent activity to show.";
  else if (ticket === null) logNote = detail?.failed ? "Couldn't load the ticket's history." : "Loading…";
  const doing = humanizeDoing(runner.doing);
  return {
    kind: "Runner",
    title: runnerShortName(runner.name),
    summary: ticket === null ? doing : `${doing}${doing === "" ? "" : " · "}#${ticket.id} ${ticket.title}`,
    facts,
    logTitle: "Recent activity",
    log: ticket === null ? [] : activityLines(ticket.timeline, ticket.id),
    logNote,
  };
}

/** The current time, refreshed every `intervalMs` so clocks keep moving. */
function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(timer);
  }, [intervalMs]);
  return now;
}

/** CSS.escape where the browser has it; runner names are plain otherwise. */
function cssEscape(value: string): string {
  return typeof CSS !== "undefined" && typeof CSS.escape === "function" ? CSS.escape(value) : value.replace(/["\\]/g, "\\$&");
}
