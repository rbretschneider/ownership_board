import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Team, TeamRunner, TeamTicket, TicketDetail } from "../../api/types";
import { useDebouncedCallback } from "../../hooks/useDebouncedCallback";
import { useEventStream } from "../../hooks/useEventStream";
import { useRunnerNames } from "../../hooks/useRunnerNames";
import { useTeam } from "../../hooks/useTeam";
import { REFETCH_DEBOUNCE_MS } from "../../lib/constants";
import { relativeTime } from "../../lib/relativeTime";
import { labelRunner, nameRunnersInText, type RunnerDirectory } from "../../lib/runnerNames";
import {
  activityLines, formatTokens, formatUSD, historyEntry, humanizeDoing, linkPath, managerTargets, runnerEngine, RUNNER_STATES,
  ticketActivity, type HistoryEntry, type LinkLine, type ManagerTarget,
} from "../../lib/team";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { PageShell } from "../common/PageShell";
import { BuildHostCard, CIRunnersCard, ManagerCard, RunnerCard, TeamBranch, TeamTicketCard } from "./TeamCards";
import { TeamPanel, type PanelContent } from "./TeamPanel";

const CLOCK_TICK_MS = 30_000;

type Selection = { kind: "manager" } | { kind: "runner"; name: string };

/**
 * The Team view (#/team): the board as an org chart. The manager on top, then
 * one branch per runner with the ticket it is working nested under it, then
 * the build hosts (a ticket only a host slot has nests in that slot) and CI
 * runners. Hovering or focusing a runner highlights its branch; tapping opens
 * details. While the manager is in a session, animated lines run from its
 * card to what it acts on: tickets, the runners holding them, a build host.
 */
export function TeamPage() {
  const api = useApi();
  const { team, error, isLoading, refetch } = useTeam();
  const debouncedRefetch = useDebouncedCallback(refetch, REFETCH_DEBOUNCE_MS);
  useEventStream(debouncedRefetch, refetch);
  const now = useNow(CLOCK_TICK_MS);
  const { directory } = useRunnerNames();
  const containerRef = useRef<HTMLDivElement>(null);
  const [hovered, setHovered] = useState<string | null>(null);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [detail, setDetail] = useState<{ ticketId: number; value: TicketDetail | null; failed: boolean } | null>(null);
  const [managerLines, setManagerLines] = useState<(LinkLine & { key: string })[]>([]);
  const targets = useMemo(() => (team === null ? [] : managerTargets(team)), [team]);
  const targetKey = targets.map((t) => `${t.kind}:${t.key}`).join("|");

  const selectedRunner = selection?.kind === "runner" ? team?.runners.find((r) => r.name === selection.name) ?? null : null;
  const highlighted = selectedRunner?.name ?? hovered;


  const ticketsById = useMemo(() => new Map((team?.tickets ?? []).map((t) => [t.id, t])), [team]);
  // A ticket nests under the runner that holds it; one no runner holds nests in its host slot.
  // A runner may hold one ticket and wait on another (released while it awaits
  // Windows), so every ticket whose holder is that runner nests under it.
  const ticketsByRunner = useMemo(() => {
    const out = new Map<string, TeamTicket[]>();
    for (const runner of team?.runners ?? []) {
      const own = (team?.tickets ?? []).filter((t) => t.holder === runner.name || (t.holder === "" && t.id === runner.ticket_id));
      out.set(runner.name, own);
    }
    return out;
  }, [team]);
  const heldIds = useMemo(() => new Set([...ticketsByRunner.values()].flat().map((t) => t.id)), [ticketsByRunner]);
  const highlightedTickets = new Set((highlighted === null ? [] : ticketsByRunner.get(highlighted) ?? []).map((t) => t.id));

  // A runner's ticket building in a host slot pairs with that slot: hovering or
  // focusing either shows a line between them, and a tap pins it (phones).
  const runnerOfTicket = useMemo(() => {
    const out = new Map<number, TeamRunner>();
    for (const runner of team?.runners ?? []) {
      for (const ticket of ticketsByRunner.get(runner.name) ?? []) out.set(ticket.id, runner);
    }
    return out;
  }, [team, ticketsByRunner]);
  const slotTicketIds = useMemo(() => new Set((team?.build_hosts ?? []).flatMap((h) => h.running.map((j) => j.ticket_id))), [team]);
  const [pairHover, setPairHover] = useState<number | null>(null);
  const [pairPinned, setPairPinned] = useState<number | null>(null);
  const togglePair = useCallback((id: number) => setPairPinned((current) => (current === id ? null : id)), []);
  const paired = pairHover ?? pairPinned;
  const pairedOnView = paired !== null && slotTicketIds.has(paired) && runnerOfTicket.has(paired) ? paired : null;
  const [pairLine, setPairLine] = useState<LinkLine | null>(null);

  const measurePair = useCallback(() => {
    const container = containerRef.current;
    if (container === null || pairedOnView === null) {
      setPairLine(null);
      return;
    }
    const from = container.querySelector(`.team-runners [data-ticket="${pairedOnView}"]`);
    const to = container.querySelector(`[data-slot-ticket="${pairedOnView}"]`);
    if (from === null || to === null) {
      setPairLine(null);
      return;
    }
    const box = container.getBoundingClientRect();
    const scale = container.offsetWidth > 0 ? box.width / container.offsetWidth : 1;
    setPairLine(linkPath(box, from.getBoundingClientRect(), to.getBoundingClientRect(), scale));
  }, [pairedOnView]);

  useLayoutEffect(() => {
    measurePair();
  }, [measurePair, team]);

  useEffect(() => {
    window.addEventListener("resize", measurePair);
    return () => window.removeEventListener("resize", measurePair);
  }, [measurePair]);

  const measureManager = useCallback(() => {
    const container = containerRef.current;
    const from = container?.querySelector("[data-manager]") ?? null;
    if (container === null || from === null || targets.length === 0) {
      setManagerLines([]);
      return;
    }
    const box = container.getBoundingClientRect();
    const scale = container.offsetWidth > 0 ? box.width / container.offsetWidth : 1;
    const lines: (LinkLine & { key: string })[] = [];
    for (const target of targets) {
      const to = container.querySelector(targetSelector(target));
      if (to !== null) lines.push({ key: `${target.kind}:${target.key}`, ...linkPath(box, from.getBoundingClientRect(), to.getBoundingClientRect(), scale) });
    }
    setManagerLines(lines);
    // targetKey stands for targets by value, so a refetch with the same targets keeps the lines.
  }, [targetKey]);

  useLayoutEffect(() => {
    measureManager();
  }, [measureManager, team]);

  useEffect(() => {
    window.addEventListener("resize", measureManager);
    return () => window.removeEventListener("resize", measureManager);
  }, [measureManager]);

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
    if (selection.kind === "manager") return managerPanel(team, directory);
    return selectedRunner === null ? null : runnerPanel(selectedRunner, detail?.ticketId === selectedRunner.ticket_id ? detail : null, now, directory);
  }, [team, selection, selectedRunner, detail, now, directory]);

  const ticketCard = (ticket: TeamTicket, isUnderRunner = false) => {
    const pairs = isUnderRunner && slotTicketIds.has(ticket.id);
    return (
      <TeamTicketCard key={ticket.id} ticket={ticket} activity={ticketActivity(ticket, team?.build_hosts ?? [], now, directory)}
        isLinked={highlightedTickets.has(ticket.id) || ticket.id === pairedOnView} now={now}
        onPairHover={pairs ? setPairHover : undefined} onPairToggle={pairs ? togglePair : undefined} />
    );
  };
  const slotOwner = (ticketId: number) => {
    const runner = runnerOfTicket.get(ticketId);
    return runner === undefined ? "" : labelRunner(runner.name, directory, runner).label;
  };

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
          {managerLines.length > 0 && (
            <svg className="team-wire team-wire--manager" aria-hidden="true" data-testid="manager-lines" key={targetKey}>
              {managerLines.map((l) => (
                <g key={l.key} data-target={l.key}>
                  <path className="team-wire__path team-wire__path--flow" d={l.d} />
                  <circle className="team-wire__end" cx={l.x2} cy={l.y2} r="4" />
                </g>
              ))}
            </svg>
          )}
          {pairLine !== null && (
            <svg className="team-wire team-wire--pair" aria-hidden="true" data-testid="pair-line" key={pairedOnView ?? ""}>
              <path className="team-wire__path" d={pairLine.d} />
              <circle className="team-wire__end" cx={pairLine.x2} cy={pairLine.y2} r="4" />
            </svg>
          )}
          <p className="team-intro">Each runner with the ticket it is working. Tap a runner for details; tap a building ticket or its slot to pair them.</p>
          <section className="team-top" aria-label="Manager">
            <ManagerCard manager={team.manager} onOpen={() => setSelection({ kind: "manager" })} />
          </section>
          {/* A hierarchy: runners claim tickets, tickets claim build slots, so the hosts sit under the runners. */}
          <section className="team-section" aria-label="Runners">
            <h2 className="team-heading">Runners</h2>
            <div className="team-runners">
              {team.runners.length === 0 && <p className="team-empty">No runners seen in the last few hours.</p>}
              {team.runners.map((runner) => {
                const own = ticketsByRunner.get(runner.name) ?? [];
                return (
                  <TeamBranch key={runner.name}>
                    <RunnerCard
                      runner={runner}
                      isActive={runner.name === highlighted}
                      onEnter={() => setHovered(runner.name)}
                      onLeave={() => setHovered((current) => (current === runner.name ? null : current))}
                      onOpen={() => setSelection({ kind: "runner", name: runner.name })}
                    />
                    {own.map((ticket) => ticketCard(ticket, true))}
                  </TeamBranch>
                );
              })}
            </div>
          </section>
          <section className="team-section" aria-label="Build hosts">
            <h2 className="team-heading">Build hosts</h2>
            {team.build_hosts.length === 0
              ? <p className="team-empty">No approved build hosts.</p>
              : (
                <div className="team-hosts">
                  {team.build_hosts.map((host) => (
                    <BuildHostCard key={host.id} host={host} now={now} renderSlotTicket={(id) => {
                      const ticket = ticketsById.get(id);
                      return ticket !== undefined && !heldIds.has(id) ? ticketCard(ticket) : null;
                    }} slotOwner={slotOwner} pairedTicket={pairedOnView} pinnedTicket={pairPinned} onPairHover={setPairHover} onPairToggle={togglePair} />
                  ))}
                </div>
              )}
          </section>
          {hasCISample && team.ci_runners !== undefined && <CIRunnersCard sample={team.ci_runners} now={now} />}
          {panel !== null && <TeamPanel content={panel} onClose={close} />}
        </div>
      )}
    </PageShell>
  );
}

function managerPanel(team: Team, directory: RunnerDirectory): PanelContent {
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
    log: [],
    history: m.sessions.map((s) => ({ at: s.at, chip: [formatUSD(s.cost_usd), s.model].filter(Boolean).join(" · "), ...namedHistory(historyEntry(s), directory) })),
    logNote: m.sessions.length === 0 ? "No manager sessions recorded yet." : undefined,
  };
}

function runnerPanel(runner: TeamRunner, detail: { value: TicketDetail | null; failed: boolean } | null, now: number, directory: RunnerDirectory): PanelContent {
  const ticket = detail?.value ?? null;
  const facts = [
    { label: "Model", value: runnerEngine(runner) },
    { label: "State", value: RUNNER_STATES[runner.state].label },
    { label: "Ticket", value: runner.ticket_id === null ? "none" : `#${runner.ticket_id}` },
    { label: "Last report", value: runner.last_seen === null ? "—" : relativeTime(runner.last_seen, new Date(now)) },
  ];
  if (runner.profile_version !== "") facts.push({ label: "Profile", value: runner.profile_version });
  if (ticket !== null) facts.push({ label: "Ticket tokens", value: formatTokens(ticket.usage?.weighted_tokens) });
  let logNote: string | undefined;
  if (runner.ticket_id === null) logNote = "No ticket held, so no recent activity to show.";
  else if (ticket === null) logNote = detail?.failed ? "Couldn't load the ticket's history." : "Loading…";
  const doing = humanizeDoing(runner.doing);
  return {
    kind: "Runner",
    title: labelRunner(runner.name, directory, runner).detail,
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

function targetSelector(target: ManagerTarget): string {
  const attribute = target.kind === "ticket" ? "data-ticket" : target.kind === "runner" ? "data-runner" : "data-host";
  return `[${attribute}="${cssEscape(target.key)}"]`;
}

/** CSS.escape where the browser has it; runner names are plain otherwise. */
function cssEscape(value: string): string {
  return typeof CSS !== "undefined" && typeof CSS.escape === "function" ? CSS.escape(value) : value.replace(/["\\]/g, "\\$&");
}

/** A manager history entry with the runners it mentions named. */
function namedHistory(entry: HistoryEntry, directory: RunnerDirectory): HistoryEntry {
  return { ...entry, headline: nameRunnersInText(entry.headline, directory), body: nameRunnersInText(entry.body, directory) };
}
