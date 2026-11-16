import type { CIRunner, RunnerState, StageId, Team, TeamBuildHost, TeamRunner, TeamTicket, TimelineEntry } from "../api/types";
import { formatElapsed } from "./buildClock";
import { modelFamily, parseWorkModel, type ModelFamily } from "./workModel";

/** "@worker/7/codex_1" reads as "codex_1"; other names stay as they are. */
export function runnerShortName(name: string): string {
  const match = /^@worker\/\d+\/(.+)$/.exec(name);
  return match?.[1] ?? name;
}

/** "Claude · opus-x" or "Codex · gpt-y": provider and model on one line. */
export function runnerEngine(runner: Pick<TeamRunner, "name" | "model">): string {
  const parsed = parseWorkModel(runner.model);
  const model = parsed?.name ?? "";
  // The runner's own name says which pool it is; the model is what it reported.
  // A Codex runner that escalated a repair to Claude says so.
  const pool = runnerFamily(runner.name);
  const modelSide = modelFamily(model);
  const family = pool ?? modelSide;
  const provider = family === "claude" ? "Claude" : family === "codex" ? "Codex" : "";
  let tail = parsed?.escalatedAfter !== undefined ? " · escalated" : "";
  if (pool !== undefined && modelSide !== undefined && modelSide !== pool) {
    tail = ` · escalated to ${modelSide === "claude" ? "Claude" : "Codex"}`;
  }
  return [provider, model].filter((part) => part !== "").join(" · ") + tail || "Model not reported";
}

/** The pool a runner belongs to, from its name: "codex_1" or "claude_2". */
export function runnerFamily(name: string): ModelFamily | undefined {
  const short = runnerShortName(name).toLowerCase();
  if (short.startsWith("codex")) return "codex";
  if (short.startsWith("claude")) return "claude";
  return undefined;
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
    case "epic":
      return { label: "Epic", tone: "grey" };
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

/** Whether a ticket in flight is moving, waiting, or blocked, in that display order. */
export type ActivityGroup = "active" | "waiting" | "blocked";

/** What a ticket in flight is doing or waiting on, in one line. */
export interface TicketActivity {
  group: ActivityGroup;
  text: string;
  tone: "accent" | "green" | "amber" | "grey";
}


/**
 * What a ticket in flight is doing: blocked (with its reason's first line),
 * on a build-host slot, held by a runner, a PR with CI running, in a build
 * queue, or waiting on a dependency, the owner or a runner.
 */
export function ticketActivity(ticket: TeamTicket, hosts: TeamBuildHost[], now: number): TicketActivity {
  if (ticket.blocked_reason.trim() !== "") {
    const reason = ticket.blocked_reason.split("\n").find((line) => line.trim() !== "")?.trim() ?? "";
    return { group: "blocked", text: `Blocked: ${clip(reason, 90)}`, tone: "amber" };
  }
  for (const host of hosts) {
    const slot = host.running.findIndex((job) => job.ticket_id === ticket.id);
    if (slot >= 0) {
      const phase = host.running[slot]?.phase || "running";
      return { group: "active", text: `${capitalize(phase)} on ${host.name} slot ${slot + 1}`, tone: "accent" };
    }
  }
  const leased = ticket.lease !== null && Date.parse(ticket.lease.expires_at) > now;
  if (leased && ticket.holder !== "" && !isAwaiting(ticket.work_model)) {
    const signal = humanizeDoing((ticket.work_model ?? "").split(" · ").slice(1).join(" · "));
    return { group: "active", text: `${runnerShortName(ticket.holder)}: ${signal || "working"}`, tone: "accent" };
  }
  const pr = ticket.pr;
  if (pr && pr.checks_total > 0 && pr.checks_done < pr.checks_total && pr.verdict !== "merged" && pr.verdict !== "closed") {
    return { group: "active", text: `PR open, CI ${pr.checks_done}/${pr.checks_total}${pr.checks_failed > 0 ? `, ${pr.checks_failed} failed` : ""}`, tone: "accent" };
  }
  for (const host of hosts) {
    const position = host.queued.findIndex((job) => job.ticket_id === ticket.id);
    if (position >= 0) {
      return { group: "waiting", text: `${ordinal(position + 1)} in ${host.name} queue`, tone: "amber" };
    }
  }
  if (ticket.build?.status === "queued") return { group: "waiting", text: "In the build queue", tone: "amber" };
  if (ticket.waiting_on.length > 0) {
    return { group: "waiting", text: `Waiting on ${ticket.waiting_on.map((id) => `#${id}`).join(", ")}`, tone: "grey" };
  }
  if (pr) {
    const text = pr.verdict === "green_owner" ? "PR green, waiting to merge"
      : pr.verdict === "repair" ? `PR needs repair${pr.reason ? ` (${pr.reason.replace(/_/g, " ")})` : ""}`
      : pr.checks_failed > 0 ? `PR open, ${pr.checks_failed} check${pr.checks_failed === 1 ? "" : "s"} failed`
      : "PR open";
    return { group: "waiting", text, tone: pr.verdict === "green_owner" ? "green" : "grey" };
  }
  if (ticket.needs_owner) return { group: "waiting", text: ticket.open_questions > 0 ? "Questions for the owner" : "Waiting for the owner", tone: "amber" };
  if (ticket.holder !== "") return { group: "waiting", text: `${runnerShortName(ticket.holder)}: ${humanizeDoing((ticket.work_model ?? "").split(" · ").slice(1).join(" · ")) || "waiting"}`, tone: "amber" };
  return { group: "waiting", text: ticket.stage === "ready" ? "Ready for a runner" : "Waiting for a runner", tone: "grey" };
}


function isAwaiting(workModel: string | undefined): boolean {
  return /\bawaiting\b/i.test(workModel ?? "");
}

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function clip(text: string, limit: number): string {
  return text.length > limit ? `${text.slice(0, limit - 1).trimEnd()}…` : text;
}

/** One line the manager's activity draws: from the manager card to a target card. */
export interface ManagerTarget {
  kind: "ticket" | "runner" | "host";
  /** The target's data attribute selector value: ticket id, runner name or host name. */
  key: string;
}

/**
 * What the working manager points at: each ticket it acts on that is on the
 * view, the runner holding that ticket, and the build host it names. Nothing
 * when it is idle or reviewing the whole board with no target.
 */
export function managerTargets(team: Pick<Team, "manager" | "tickets" | "runners" | "build_hosts">): ManagerTarget[] {
  const activity = team.manager.activity;
  if (!activity || activity.state !== "working") return [];
  const targets: ManagerTarget[] = [];
  for (const id of activity.ticket_ids) {
    if (!team.tickets.some((t) => t.id === id)) continue;
    targets.push({ kind: "ticket", key: String(id) });
    const runner = team.runners.find((r) => r.ticket_id === id);
    if (runner && !targets.some((t) => t.kind === "runner" && t.key === runner.name)) targets.push({ kind: "runner", key: runner.name });
  }
  if (activity.host !== "" && team.build_hosts.some((h) => h.name === activity.host)) targets.push({ kind: "host", key: activity.host });
  return targets;
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

/** A CI sample older than this is shown as stale: the sweep runs every 5 minutes. */
export const CI_SAMPLE_STALE_MS = 15 * 60_000;

/** A CI runner's state on its row: busy, idle or offline. */
export function ciRunnerState(runner: CIRunner): { text: string; tone: "accent" | "green" | "grey" } {
  if (runner.status !== "online") return { text: "offline", tone: "grey" };
  return runner.busy ? { text: "busy", tone: "accent" } : { text: "idle", tone: "green" };
}

/** The labels worth showing: every label but the generic "self-hosted". */
export function ciRunnerLabels(runner: CIRunner): string {
  return runner.labels.filter((label) => label.toLowerCase() !== "self-hosted").join(", ");
}

/** The model chip a runner card shows, as the Board card does: the model, ranked when escalated. */
export function runnerModelChip(runner: Pick<TeamRunner, "model">): { name: string; escalatedAfter?: number } | undefined {
  return parseWorkModel(runner.model);
}

/** The runner's pool and, when it escalated to the other family, that too: "Codex runner · escalated to Claude". */
export function runnerPool(runner: Pick<TeamRunner, "name" | "model">): string {
  const used = modelFamily(parseWorkModel(runner.model)?.name);
  const pool = runnerFamily(runner.name) ?? used;
  const label = (family: ModelFamily | undefined) => (family === "claude" ? "Claude" : family === "codex" ? "Codex" : "");
  const base = label(pool) || "Agent";
  return used !== undefined && pool !== undefined && used !== pool ? `${base} runner · escalated to ${label(used)}` : `${base} runner`;
}

/** One manager history entry, ready to show: a headline and a body to render. */
export interface HistoryEntry {
  headline: string;
  /** Markdown for structured summaries; plain text (one item per line) for older ones. */
  body: string;
  isMarkdown: boolean;
}

/**
 * Splits a session summary into its headline and body. Structured summaries
 * (a HEADLINE line, then markdown sections) render as markdown. Older ones
 * fall back to the first sentence as the headline and the rest as plain text,
 * with " | " separators turned into line breaks.
 */
export function historyEntry(session: { summary: string; headline?: string; structured?: boolean }): HistoryEntry {
  const lines = session.summary.split("\n");
  const isLabel = (line: string) => /^(board review|manager summary)$/i.test(line.replace(/[*#`]/g, "").trim());
  if (session.structured === true) {
    const at = lines.findIndex((line) => /^[\s#*>-]*headline:/i.test(line));
    const body = lines.slice(at + 1).filter((line) => !isLabel(line)).join("\n").trim();
    return { headline: session.headline ?? "", body, isMarkdown: true };
  }
  const text = lines.filter((line) => !isLabel(line)).join("\n").trim();
  const headline = session.headline ?? firstSentenceOf(text);
  const plain = headline.endsWith("…") ? headline.slice(0, -1) : headline;
  const rest = plain !== "" && text.startsWith(plain) ? text.slice(plain.length) : text;
  const body = rest.split(/\s+\|\s+/).join("\n").replace(/\n{3,}/g, "\n\n").trim();
  return { headline, body, isMarkdown: false };
}

function firstSentenceOf(text: string): string {
  const line = text.split("\n").find((l) => l.trim() !== "")?.trim() ?? "";
  const end = line.search(/[.!?](\s|$)/);
  return end >= 0 ? line.slice(0, end + 1) : line;
}
