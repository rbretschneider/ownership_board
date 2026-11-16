import type { RunnerState, StageId, TeamBuildHost, TeamRunner, TeamTicket, TimelineEntry } from "../api/types";
import { formatElapsed } from "./buildClock";
import { modelFamily, parseWorkModel } from "./workModel";

/** "@worker/7/codex_1" reads as "codex_1"; other names stay as they are. */
export function runnerShortName(name: string): string {
  const match = /^@worker\/\d+\/(.+)$/.exec(name);
  return match?.[1] ?? name;
}

/** "Claude · opus-x" or "Codex · gpt-y": provider and model on one line. */
export function runnerEngine(runner: Pick<TeamRunner, "name" | "model">): string {
  const model = parseWorkModel(runner.model)?.name ?? "";
  const family = modelFamily(model) ?? modelFamily(runnerShortName(runner.name).replace(/[_-]/g, " "));
  const provider = family === "claude" ? "Claude" : family === "codex" ? "Codex" : "";
  const escalated = parseWorkModel(runner.model)?.escalatedAfter !== undefined ? " · escalated" : "";
  return [provider, model].filter((part) => part !== "").join(" · ") + escalated || "Model not reported";
}

const PHASES: Record<string, string> = {
  queued: "queued", paused: "paused", preparing: "preparing", building: "building", testing: "testing", verifying: "checking",
};

/**
 * A worker's raw signal as one readable line: build-server markers become
 * "Windows x64 build queued", "· awaiting X" becomes "awaiting X".
 */
export function humanizeDoing(doing: string): string {
  const build = /build-server:([a-z]+):([^·]+)/.exec(doing);
  if (build?.[1] && build[2]) {
    return `${build[2].trim()} build ${PHASES[build[1]] ?? build[1]}`;
  }
  const text = doing.replace(/\s*·\s*/g, " · ").trim();
  return text === "" ? "" : text.charAt(0).toUpperCase() + text.slice(1);
}

/** Display label and tone for a runner state. */
export const RUNNER_STATES: Record<RunnerState, { label: string; tone: "accent" | "amber" | "grey" }> = {
  working: { label: "Working", tone: "accent" },
  waiting: { label: "Waiting", tone: "amber" },
  idle: { label: "Idle", tone: "grey" },
};

/** The role a ticket's stage plays in the team: who works it now. */
export function ticketRole(stage: StageId): { label: string; tone: "accent" | "green" | "amber" | "grey" } {
  switch (stage) {
    case "refine":
    case "spec_review":
      return { label: "Refine", tone: "grey" };
    case "ready":
    case "design":
    case "design_review":
      return { label: "Design", tone: "amber" };
    case "build":
      return { label: "Engineer", tone: "accent" };
    case "pr":
      return { label: "Review", tone: "green" };
    default:
      return { label: "Deploy", tone: "green" };
  }
}

/** Where a ticket in flight is: on a host slot, in a host queue, or with its runner. */
export function ticketWhere(ticket: TeamTicket, hosts: TeamBuildHost[]): string {
  for (const host of hosts) {
    const slot = host.running.findIndex((job) => job.ticket_id === ticket.id);
    if (slot >= 0) {
      const phase = host.running[slot]?.phase;
      return `${host.name} slot ${slot + 1}${phase ? ` · ${phase}` : ""}`;
    }
    const position = host.queued.findIndex((job) => job.ticket_id === ticket.id);
    if (position >= 0) return `${host.name} queue · ${ordinal(position + 1)}`;
  }
  if (ticket.build?.status === "queued") return "build queue";
  if (ticket.holder !== "") return runnerShortName(ticket.holder);
  return ticket.waiting_on.length > 0 ? `waits on ${ticket.waiting_on.map((id) => `#${id}`).join(", ")}` : "unassigned";
}

/** "2h 08m on host", "queued 1h 06m" or "—": how long the ticket's build has run or waited. */
export function ticketClock(ticket: TeamTicket, now: number): string {
  const build = ticket.build;
  if (!build) return "—";
  const started = build.started_at === null ? NaN : Date.parse(build.started_at);
  if (build.status === "running" && Number.isFinite(started)) return `${formatElapsed(now - started)} on host`;
  const queued = Date.parse(build.queued_at);
  return Number.isFinite(queued) ? `queued ${formatElapsed(now - queued)}` : "—";
}

/** "1.9M tok", "820k tok", "0 tok". */
export function formatTokens(tokens: number | undefined): string {
  const n = tokens ?? 0;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M tok`;
  if (n >= 1_000) return `${Math.round(n / 1_000)}k tok`;
  return `${n} tok`;
}

/** "$1.94". */
export function formatUSD(value: number): string {
  return `$${value.toFixed(2)}`;
}

/** "14:05": a local clock time for logs. */
export function clockTime(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  return `${String(at.getHours()).padStart(2, "0")}:${String(at.getMinutes()).padStart(2, "0")}`;
}

/** "slot busy" text for a host: "2 of 2 slots busy", "idle", "offline". */
export function hostLoad(host: TeamBuildHost): { text: string; tone: "accent" | "amber" | "green" | "grey" } {
  if (!host.online) return { text: "offline", tone: "grey" };
  if (!host.enabled) return { text: "disabled", tone: "grey" };
  const busy = host.running.length;
  const slots = Math.max(host.max_parallel, busy);
  if (busy === 0) return { text: `${slots} ${slots === 1 ? "slot" : "slots"} free`, tone: "green" };
  return { text: `${busy} of ${slots} slots busy`, tone: busy >= slots ? "amber" : "accent" };
}

/** One line of a runner's recent activity, from a ticket timeline entry. */
export interface ActivityLine {
  at: string;
  text: string;
  tag: string;
  tone: "accent" | "green" | "amber" | "grey";
}

const ACTIVITY_LIMIT = 8;

/** The newest timeline entries of a ticket as activity lines, newest first. */
export function activityLines(entries: TimelineEntry[], ticketId: number): ActivityLine[] {
  return entries
    .slice(-ACTIVITY_LIMIT)
    .reverse()
    .map((entry) => ({ at: entry.at, ...describeEntry(entry, ticketId) }));
}

function describeEntry(entry: TimelineEntry, ticketId: number): Omit<ActivityLine, "at"> {
  const ref = `#${ticketId}`;
  const model = typeof entry.data["model"] === "string" ? entry.data["model"] : "";
  switch (entry.type) {
    case "edited":
      return model !== ""
        ? { text: `${ref} ${humanizeDoing(model.split(" · ").slice(1).join(" · ")) || "reported its model"} (${parseWorkModel(model)?.name ?? model})`, tag: "model", tone: "accent" }
        : { text: `${ref} edited`, tag: "edit", tone: "grey" };
    case "stage":
      return { text: `${ref} moved from ${String(entry.data["from"])} to ${String(entry.data["to"])}`, tag: "stage", tone: "green" };
    case "claimed":
      return { text: `${ref} claimed${typeof entry.data["action"] === "string" ? ` to ${entry.data["action"]}` : ""}`, tag: "claim", tone: "accent" };
    case "released":
      return { text: `${ref} released`, tag: "release", tone: "grey" };
    case "comment":
      return { text: firstLine(entry.body), tag: entry.actor.kind === "agent" ? "comment" : "owner", tone: "grey" };
    case "blocked":
      return { text: `${ref} blocked`, tag: "blocked", tone: "amber" };
    case "unblocked":
      return { text: `${ref} unblocked`, tag: "unblocked", tone: "green" };
    case "questions":
      return { text: `${ref} asked the owner`, tag: "question", tone: "amber" };
    case "work_link":
      return { text: `${ref} linked ${entry.data["kind"] === "commit" ? "a commit" : "a pull request"}`, tag: "commit", tone: "green" };
    default:
      return { text: `${ref} ${String(entry.type).replace(/_/g, " ")}`, tag: String(entry.type).replace(/_/g, " "), tone: "grey" };
  }
}

const LINE_LIMIT = 140;

function firstLine(body: string): string {
  const line = body.split("\n").find((l) => l.trim() !== "")?.trim() ?? "";
  return line.length > LINE_LIMIT ? `${line.slice(0, LINE_LIMIT - 1)}…` : line;
}

function ordinal(n: number): string {
  const suffix = n % 100 >= 11 && n % 100 <= 13 ? "th" : (["th", "st", "nd", "rd"][n % 10] ?? "th");
  return `${n}${suffix}`;
}

/** A rectangle as getBoundingClientRect reports it. */
export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

/** A curved link from the bottom of one box to the top of another. */
export interface LinkLine {
  d: string;
  x2: number;
  y2: number;
}

/**
 * The curved line from a runner card to its ticket, in the container's own
 * (unscaled) coordinates. `scale` is the container's rendered width over its
 * layout width, so a CSS transform on an ancestor does not offset the line.
 * When the ticket sits above the runner (wrapped layouts), the line runs
 * from the runner's top to the ticket's bottom instead.
 */
export function linkPath(container: Box, from: Box, to: Box, scale = 1): LinkLine {
  const s = scale > 0 && Number.isFinite(scale) ? scale : 1;
  const isBelow = to.top >= from.top + from.height;
  const x1 = (from.left + from.width / 2 - container.left) / s;
  const y1 = ((isBelow ? from.top + from.height : from.top) - container.top) / s;
  const x2 = (to.left + to.width / 2 - container.left) / s;
  const y2 = ((isBelow ? to.top : to.top + to.height) - container.top) / s;
  const bend = Math.max(30, Math.abs(y2 - y1) / 2) * (isBelow ? 1 : -1);
  const r = (n: number) => Math.round(n * 10) / 10;
  return { d: `M${r(x1)} ${r(y1)} C ${r(x1)} ${r(y1 + bend)}, ${r(x2)} ${r(y2 - bend)}, ${r(x2)} ${r(y2)}`, x2: r(x2), y2: r(y2) };
}
