import type { Repo, TicketSummary } from "../api/types";
import { isCompletedStage } from "./stages";
import { cardActivity, hasLiveLease } from "./activity";
import { currentStepIndex, onWindows } from "./progress";

/*
 * The flow view (#/flow): tickets drift left to right as they near merged, in
 * one swimlane per repository. Tickets still waiting on other tickets sit in
 * queue columns left of the flow, one column per hand-off, so every
 * dependency line runs rightwards to the work it waits on. See docs/flow.md.
 */

/** A column of the flow, after the queue. */
export type FlowStageId = "shaping" | "ready" | "design" | "build" | "windows" | "review" | "pr" | "merged";

/** The flow columns in order, left to right. */
export const FLOW_STAGES: readonly { id: FlowStageId; label: string; about: string }[] = [
  { id: "shaping", label: "Shaping", about: "Refining the idea or awaiting plan approval" },
  { id: "ready", label: "Ready", about: "Nothing left to wait for; an agent can start" },
  { id: "design", label: "Design", about: "Planning delivery or awaiting approach approval" },
  { id: "build", label: "Build", about: "Writing the code" },
  { id: "windows", label: "Windows", about: "Verification on the Windows PC" },
  { id: "review", label: "Review", about: "Fresh-context adversarial review" },
  { id: "pr", label: "PR", about: "Pull request open, delivery checks running" },
  { id: "merged", label: "Merged", about: "Merged in the last 7 days" },
];

/** Node size and spacing in CSS pixels; flow.css sizes nodes from the same numbers. */
export const FLOW_SIZES = {
  nodeWidth: 164,
  nodeHeight: 78,
  columnGap: 40,
  rowGap: 10,
  collapsedWidth: 28,
  headerHeight: 36,
  laneHeadHeight: 34,
  lanePadBottom: 14,
  padX: 12,
} as const;

/** Merged tickets each lane shows; older ones in the window are counted. */
export const MERGED_SHOWN = 3;

/** How far back the merged column looks. */
export const MERGED_WINDOW_MS = 7 * 24 * 60 * 60 * 1000;

const DAY_MS = 24 * 60 * 60 * 1000;

/** Retries from which a ticket counts as a loop worth a look, and as a hot one. */
export const RETRY_WARM = 5;
export const RETRY_HOT = 15;

/** Which flow column a ticket's stage puts it in (ignoring dependencies). */
export function flowStage(t: Pick<TicketSummary, "stage" | "work_model">): FlowStageId {
  switch (t.stage) {
    case "refine":
    case "spec_review":
      return "shaping";
    case "ready":
      return "ready";
    case "design":
    case "design_review":
      return "design";
    case "build": {
      if (onWindows(t)) return "windows";
      return currentStepIndex(t) === 2 ? "review" : "build";
    }
    case "pr":
      return "pr";
    case "deploying":
    case "deployed":
    case "done":
      return "merged";
  }
}

/** A ticket's bounce-backs: failed Windows runs, review fixes and PR repairs. */
export interface Retries {
  total: number;
  /** e.g. "25 failed Windows runs · 12 review fixes"; "" when none. */
  detail: string;
}

/** Retries from the board's derived attempt counts (none when the server sends no counts). */
export function retries(t: Pick<TicketSummary, "attempts">): Retries {
  const a = t.attempts;
  if (a === undefined) return { total: 0, detail: "" };
  const parts = [
    a.test_failures > 0 && `${a.test_failures} failed Windows run${a.test_failures === 1 ? "" : "s"}`,
    a.review_fixes > 0 && `${a.review_fixes} review fix${a.review_fixes === 1 ? "" : "es"}`,
    a.pr_repairs > 0 && `${a.pr_repairs} PR repair${a.pr_repairs === 1 ? "" : "s"}`,
  ].filter((p): p is string => typeof p === "string");
  return { total: a.test_failures + a.review_fixes + a.pr_repairs, detail: parts.join(" · ") };
}

/** How a node reads at a glance; drives its colour. */
export type NodeTone = "working" | "owner" | "stuck" | "windows" | "waiting" | "ready" | "idle" | "done";

/** The node's tone and its status word (the board card's own activity label). */
export function nodeStatus(t: TicketSummary, now: number): { tone: NodeTone; label: string } {
  const activity = cardActivity(t, now);
  const label = activity.label;
  if (isCompletedStage(t.stage)) return { tone: "done", label };
  if (t.blocked_reason !== "") return { tone: "stuck", label };
  if (t.needs_owner) return { tone: "owner", label };
  if (activity.buildServer === true) return { tone: label.startsWith("Paused") ? "stuck" : "windows", label };
  if (t.waiting_on.length > 0) return { tone: "waiting", label };
  if (label === "Awaiting Windows" || label === "Verification needed") return { tone: "windows", label };
  if (hasLiveLease(t, now)) return { tone: "working", label };
  if (t.stage === "ready" || t.stage === "refine") return { tone: "ready", label: t.stage === "ready" ? "Ready" : label };
  return { tone: "idle", label: `${label} · idle` };
}

/** Open tickets and the dependencies between them. */
export interface FlowGraph {
  /** Every ticket on the board, by id. */
  byId: Map<number, TicketSummary>;
  /** Open tickets' unmerged dependencies that are on the board. */
  deps: Map<number, number[]>;
  /** The reverse of `deps`. */
  dependents: Map<number, number[]>;
  /** Hand-offs before a ticket can start: 0 for startable or in-flight work. */
  depth: Map<number, number>;
}

/**
 * Builds the dependency graph from `waiting_on`, which lists only unmerged
 * dependencies, so every edge is live blocking work. A dependency that is not
 * on the board still counts as one hand-off. Cycles (which the server should
 * never allow) are cut rather than looping.
 */
export function buildGraph(tickets: readonly TicketSummary[]): FlowGraph {
  const byId = new Map(tickets.map((t) => [t.id, t]));
  const deps = new Map<number, number[]>();
  const dependents = new Map<number, number[]>();
  for (const t of tickets) {
    if (isCompletedStage(t.stage)) continue;
    const present = t.waiting_on.filter((id) => byId.has(id) && !isCompletedStage(byId.get(id)!.stage));
    deps.set(t.id, present);
    for (const dep of present) {
      dependents.set(dep, [...(dependents.get(dep) ?? []), t.id]);
    }
  }
  const depth = new Map<number, number>();
  const visiting = new Set<number>();
  const depthOf = (id: number): number => {
    const known = depth.get(id);
    if (known !== undefined) return known;
    const t = byId.get(id);
    if (t === undefined || t.waiting_on.length === 0 || visiting.has(id)) return 0;
    visiting.add(id);
    const value = 1 + Math.max(0, ...(deps.get(id) ?? []).map(depthOf));
    visiting.delete(id);
    depth.set(id, value);
    return value;
  };
  for (const id of deps.keys()) depth.set(id, depthOf(id));
  return { byId, deps, dependents, depth };
}

function reach(start: number, next: Map<number, number[]>): Set<number> {
  const seen = new Set<number>();
  const stack = [...(next.get(start) ?? [])];
  while (stack.length > 0) {
    const id = stack.pop() as number;
    if (seen.has(id) || id === start) continue;
    seen.add(id);
    stack.push(...(next.get(id) ?? []));
  }
  return seen;
}

/** Everything a ticket transitively waits on. */
export function upstreamOf(graph: FlowGraph, id: number): Set<number> {
  return reach(id, graph.deps);
}

/** Everything that transitively waits on a ticket: what landing it unlocks. */
export function downstreamOf(graph: FlowGraph, id: number): Set<number> {
  return reach(id, graph.dependents);
}

/**
 * The longest chain from a ticket back to the work it is ultimately stuck
 * behind: [ticket, its deepest dependency, …, head]. At each step the deeper
 * dependency wins, then the one gating more work, then the lower id.
 */
export function criticalPath(graph: FlowGraph, targetId: number, gates?: Map<number, number>): number[] {
  const path = [targetId];
  const seen = new Set(path);
  let current = targetId;
  for (;;) {
    const options = (graph.deps.get(current) ?? []).filter((id) => !seen.has(id));
    if (options.length === 0) return path;
    options.sort((a, b) =>
      (graph.depth.get(b) ?? 0) - (graph.depth.get(a) ?? 0)
      || (gates?.get(b) ?? 0) - (gates?.get(a) ?? 0)
      || a - b);
    current = options[0] as number;
    seen.add(current);
    path.push(current);
  }
}

/** How many tickets transitively wait on each open ticket. */
export function gateCounts(graph: FlowGraph): Map<number, number> {
  const counts = new Map<number, number>();
  for (const id of graph.deps.keys()) counts.set(id, downstreamOf(graph, id).size);
  return counts;
}

/** A column of the canvas. */
export interface FlowColumn {
  key: string;
  label: string;
  about: string;
  /** Queue columns carry their hand-off count. */
  depth: number | null;
  x: number;
  width: number;
  /** Empty in every lane: drawn as a thin strip. */
  isCollapsed: boolean;
  count: number;
}

/** A placed ticket. */
export interface FlowNode {
  ticket: TicketSummary;
  laneKey: string;
  columnKey: string;
  /** Canvas coordinates of the node's top-left corner. */
  x: number;
  y: number;
}

/** One repository's swimlane. */
export interface FlowLane {
  key: string;
  name: string;
  /** "" or `#rrggbb`. */
  color: string;
  y: number;
  height: number;
  open: number;
  queued: number;
  inFlight: number;
  /** Merged in the window, including those not drawn. */
  merged: number;
  /** The lane's default milestone: its deepest queued ticket, or null. */
  milestoneId: number | null;
  /** Critical path to that milestone, milestone first. */
  criticalPath: number[];
}

/** A dependency line: `from` waits on `to`. */
export interface FlowEdge {
  from: number;
  to: number;
}

/** Everything the canvas draws. */
export interface FlowLayout {
  graph: FlowGraph;
  gates: Map<number, number>;
  columns: FlowColumn[];
  lanes: FlowLane[];
  nodes: Map<number, FlowNode>;
  edges: FlowEdge[];
  width: number;
  height: number;
}

const NO_REPO_KEY = "none";

/** The swimlane key for a ticket: its repo id, or "none". */
export function laneKeyOf(t: Pick<TicketSummary, "repo_id">): string {
  return t.repo_id === null ? NO_REPO_KEY : String(t.repo_id);
}

/**
 * The simple flow, for an owner who cares that work moves rather than how it
 * is worked: everything an agent does between starting and opening a PR is
 * one "In progress" column.
 */
export const SIMPLE_STAGES: readonly { id: string; label: string; about: string; covers: readonly FlowStageId[] }[] = [
  { id: "next", label: "Up next", about: "Free to start: being shaped or ready for an agent", covers: ["shaping", "ready"] },
  { id: "doing", label: "In progress", about: "Design, build, Windows verification and review", covers: ["design", "build", "windows", "review"] },
  { id: "pr", label: "PR", about: "Pull request open, delivery checks running", covers: ["pr"] },
  { id: "merged", label: "Merged", about: "Merged in the last 7 days", covers: ["merged"] },
];

/** Which column a ticket's stage puts it in, in the simple or the detailed flow. */
export function stageColumnKey(t: Pick<TicketSummary, "stage" | "work_model">, isDetailed: boolean): string {
  const stage = flowStage(t);
  return isDetailed ? stage : SIMPLE_STAGES.find((s) => s.covers.includes(stage))?.id ?? stage;
}

function columnKeyOf(t: TicketSummary, graph: FlowGraph, isDetailed: boolean): string {
  const depth = graph.depth.get(t.id) ?? 0;
  return !isCompletedStage(t.stage) && depth > 0 ? `queue-${depth}` : stageColumnKey(t, isDetailed);
}

/** A repo the filter can show or hide. */
export interface LaneOption {
  key: string;
  name: string;
  color: string;
  open: number;
}

/** Repos with open work, busiest first: the filter's choices. */
export function laneOptions(tickets: readonly TicketSummary[], repos: readonly Repo[]): LaneOption[] {
  const repoById = new Map(repos.map((r) => [r.id, r]));
  const open = new Map<string, number>();
  for (const t of tickets) {
    if (!isCompletedStage(t.stage)) open.set(laneKeyOf(t), (open.get(laneKeyOf(t)) ?? 0) + 1);
  }
  return [...open.entries()]
    .map(([key, count]) => ({
      key,
      name: laneName(key, repoById),
      color: key === NO_REPO_KEY ? "" : repoById.get(Number(key))?.color ?? "",
      open: count,
    }))
    .sort((a, b) => b.open - a.open || a.name.localeCompare(b.name));
}

/** How to draw the flow. */
export interface FlowOptions {
  /** Split "In progress" into design, build, Windows and review. */
  isDetailed?: boolean;
  /** Lanes to show; null or absent shows every repo with open work. */
  laneKeys?: ReadonlySet<string> | null;
}

/**
 * Places every open ticket, plus each lane's recent merges, on the canvas.
 * Flow columns keep the owner's own board order (the order GET /board
 * returns). Columns are filled right to left, so each queue column can order
 * its tickets by the average height of what they wait on (ties in board
 * order) and push overlapping nodes down: chains read as straight threads.
 */
export function layoutFlow(tickets: readonly TicketSummary[], repos: readonly Repo[], now: number, options: FlowOptions = {}): FlowLayout {
  const S = FLOW_SIZES;
  const isDetailed = options.isDetailed ?? false;
  const laneKeys = options.laneKeys ?? null;
  const graph = buildGraph(tickets);
  const gates = gateCounts(graph);
  const repoById = new Map(repos.map((r) => [r.id, r]));
  const rank = new Map(tickets.map((t, index) => [t.id, index]));
  const byRank = (a: TicketSummary, b: TicketSummary) => (rank.get(a.id) ?? 0) - (rank.get(b.id) ?? 0);

  // Which tickets are drawn: open ones, and the newest merges per lane.
  const byLane = new Map<string, TicketSummary[]>();
  for (const t of tickets) {
    const key = laneKeyOf(t);
    byLane.set(key, [...(byLane.get(key) ?? []), t]);
  }
  interface LaneDraft { key: string; drawn: TicketSummary[]; open: TicketSummary[]; merged: number }
  const drafts: LaneDraft[] = [];
  for (const [key, laneTickets] of byLane) {
    const open = laneTickets.filter((t) => !isCompletedStage(t.stage));
    if (open.length === 0 || (laneKeys !== null && !laneKeys.has(key))) continue;
    const merged = laneTickets
      .filter((t) => isCompletedStage(t.stage) && now - Date.parse(t.lane_changed_at) <= MERGED_WINDOW_MS)
      .sort((a, b) => Date.parse(b.lane_changed_at) - Date.parse(a.lane_changed_at));
    drafts.push({ key, drawn: [...open, ...merged.slice(0, MERGED_SHOWN)], open, merged: merged.length });
  }
  drafts.sort((a, b) => b.open.length - a.open.length || laneName(a.key, repoById).localeCompare(laneName(b.key, repoById)));

  // Columns: deepest queue on the left, then the flow.
  const maxDepth = Math.max(0, ...drafts.flatMap((d) => d.open.map((t) => graph.depth.get(t.id) ?? 0)));
  const columnDefs: { key: string; label: string; about: string; depth: number | null }[] = [];
  for (let d = maxDepth; d >= 1; d--) {
    columnDefs.push({
      key: `queue-${d}`,
      label: `Queued +${d}`,
      about: d === 1 ? "Can start once one more ticket lands" : `Can start after ${d} more hand-offs`,
      depth: d,
    });
  }
  for (const s of isDetailed ? FLOW_STAGES : SIMPLE_STAGES) columnDefs.push({ key: s.id, label: s.label, about: s.about, depth: null });
  const counts = new Map<string, number>();
  for (const draft of drafts) {
    for (const t of draft.drawn) {
      const key = columnKeyOf(t, graph, isDetailed);
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
  }
  let x = S.padX;
  const columns: FlowColumn[] = columnDefs.map((def) => {
    const count = counts.get(def.key) ?? 0;
    const isCollapsed = count === 0;
    const width = isCollapsed ? S.collapsedWidth : S.nodeWidth;
    const column = { ...def, x, width, isCollapsed, count };
    x += width + S.columnGap;
    return column;
  });
  const width = x - S.columnGap + S.padX;

  // Lanes, each filled right to left.
  const nodes = new Map<number, FlowNode>();
  const lanes: FlowLane[] = [];
  let laneY = S.headerHeight;
  for (const draft of drafts) {
    const local = new Map<number, number>();
    let bottom = 0;
    for (const column of [...columns].reverse()) {
      const members = draft.drawn.filter((t) => columnKeyOf(t, graph, isDetailed) === column.key);
      if (members.length === 0) continue;
      const desired = new Map<number, number>();
      if (column.depth !== null) {
        for (const t of members) {
          const ys = (graph.deps.get(t.id) ?? []).map((id) => local.get(id)).filter((y): y is number => y !== undefined);
          desired.set(t.id, ys.length > 0 ? ys.reduce((sum, y) => sum + y, 0) / ys.length : 0);
        }
        members.sort((a, b) => (desired.get(a.id) ?? 0) - (desired.get(b.id) ?? 0) || byRank(a, b));
      } else if (column.key === "merged") {
        members.sort((a, b) => Date.parse(b.lane_changed_at) - Date.parse(a.lane_changed_at));
      } else {
        members.sort(byRank);
      }
      let nextFree = 0;
      for (const t of members) {
        const y = Math.max(desired.get(t.id) ?? 0, nextFree);
        local.set(t.id, y);
        nextFree = y + S.nodeHeight + S.rowGap;
        bottom = Math.max(bottom, y + S.nodeHeight);
        nodes.set(t.id, { ticket: t, laneKey: draft.key, columnKey: column.key, x: column.x, y: laneY + S.laneHeadHeight + y });
      }
    }
    const height = S.laneHeadHeight + bottom + S.lanePadBottom;
    const queued = draft.open.filter((t) => (graph.depth.get(t.id) ?? 0) > 0);
    const milestone = [...queued].sort((a, b) =>
      (graph.depth.get(b.id) ?? 0) - (graph.depth.get(a.id) ?? 0)
      || upstreamOf(graph, b.id).size - upstreamOf(graph, a.id).size
      || b.id - a.id)[0];
    const repo = draft.key === NO_REPO_KEY ? undefined : repoById.get(Number(draft.key));
    lanes.push({
      key: draft.key,
      name: laneName(draft.key, repoById),
      color: repo?.color ?? "",
      y: laneY,
      height,
      open: draft.open.length,
      queued: queued.length,
      inFlight: draft.open.filter((t) => !["refine", "spec_review", "ready"].includes(t.stage)).length,
      merged: draft.merged,
      milestoneId: milestone?.id ?? null,
      criticalPath: milestone === undefined ? [] : criticalPath(graph, milestone.id, gates),
    });
    laneY += height;
  }

  const edges: FlowEdge[] = [];
  for (const [from, list] of graph.deps) {
    if (!nodes.has(from)) continue;
    for (const to of list) {
      if (nodes.has(to)) edges.push({ from, to });
    }
  }
  return { graph, gates, columns, lanes, nodes, edges, width, height: laneY };
}

function laneName(key: string, repoById: Map<number, Repo>): string {
  return key === NO_REPO_KEY ? "No repo" : repoById.get(Number(key))?.name ?? `repo ${key}`;
}

/** A ticket an insight names, with why it is there. */
export interface Callout {
  ticket: TicketSummary;
  note: string;
  tone: NodeTone | "warm";
}

/** Where open work is stuck, per flow column. */
export interface StuckStage {
  key: string;
  label: string;
  /** Stuck tickets: blocked, paused, idle, or queued behind the Windows PC. */
  tickets: Callout[];
}

/** The owner's at-a-glance summary: the four questions plus a forecast. */
export interface FlowInsights {
  /** Where is the pipeline stuck? Columns with stuck work, worst first. */
  stuck: StuckStage[];
  /** What can I help remove? Things only the owner can act on, then the biggest gate. */
  help: Callout[];
  /** Is progress being made? */
  progress: { working: number; merged24h: number; merged7d: number; lastMergeAt: number | null; idle: number };
  /** Are tokens being wasted? Retries burn agent turns without moving work. */
  waste: { totalRetries: number; totalPasses: number; hotSpots: Callout[] };
}

/** Most entries any insight lists. */
export const INSIGHT_LIMIT = 4;

/** Why a stuck ticket is stuck, or null when it is not. */
function stuckNote(t: TicketSummary, now: number): Callout | null {
  const status = nodeStatus(t, now);
  if (t.blocked_reason !== "") return { ticket: t, note: `Blocked: ${t.blocked_reason}`, tone: "stuck" };
  if (status.tone === "stuck") return { ticket: t, note: status.label, tone: "stuck" };
  if (status.tone === "idle") return { ticket: t, note: status.label, tone: "idle" };
  if (status.tone === "windows" && !status.label.startsWith("Checking") && !status.label.startsWith("Testing")
    && !status.label.startsWith("Building") && !status.label.startsWith("Preparing")) {
    return { ticket: t, note: status.label, tone: "windows" };
  }
  return null;
}

/** The summary cards above the canvas. */
export function flowInsights(tickets: readonly TicketSummary[], gates: Map<number, number>, now: number): FlowInsights {
  const open = tickets.filter((t) => !isCompletedStage(t.stage));
  const gateNote = (t: TicketSummary) => ((gates.get(t.id) ?? 0) > 0 ? ` · gates ${gates.get(t.id)}` : "");

  const byStage = new Map<string, Callout[]>();
  for (const t of open) {
    if (t.waiting_on.length > 0) continue;
    const callout = stuckNote(t, now);
    if (callout === null) continue;
    const key = flowStage(t);
    byStage.set(key, [...(byStage.get(key) ?? []), { ...callout, note: callout.note + gateNote(t) }]);
  }
  const stuck = [...byStage.entries()]
    .map(([key, list]) => ({
      key,
      label: FLOW_STAGES.find((s) => s.id === key)?.label ?? key,
      tickets: list.sort((a, b) => (gates.get(b.ticket.id) ?? 0) - (gates.get(a.ticket.id) ?? 0) || a.ticket.id - b.ticket.id),
    }))
    .sort((a, b) => b.tickets.length - a.tickets.length
      || b.tickets.reduce((s, c) => s + (gates.get(c.ticket.id) ?? 0), 0) - a.tickets.reduce((s, c) => s + (gates.get(c.ticket.id) ?? 0), 0));

  const help: Callout[] = [];
  for (const t of open.filter((x) => x.needs_owner).sort((a, b) => a.id - b.id)) {
    const note = t.open_questions > 0
      ? `Answer ${t.open_questions} question${t.open_questions === 1 ? "" : "s"}`
      : t.blocked_reason !== "" ? `Unblock: ${t.blocked_reason}` : `Review: ${nodeStatus(t, now).label}`;
    help.push({ ticket: t, note: note + gateNote(t), tone: "owner" });
  }
  for (const t of open) {
    if (help.some((h) => h.ticket.id === t.id)) continue;
    const status = nodeStatus(t, now);
    if (t.blocked_reason !== "") help.push({ ticket: t, note: `Unblock: ${t.blocked_reason}${gateNote(t)}`, tone: "stuck" });
    else if (status.label.startsWith("Paused")) help.push({ ticket: t, note: `Paused on the Windows PC; resume or rescope${gateNote(t)}`, tone: "stuck" });
  }
  const topGate = open
    .filter((t) => t.waiting_on.length === 0 && (gates.get(t.id) ?? 0) > 0)
    .sort((a, b) => (gates.get(b.id) ?? 0) - (gates.get(a.id) ?? 0) || a.id - b.id)[0];
  if (topGate !== undefined && !help.some((h) => h.ticket.id === topGate.id)) {
    help.push({ ticket: topGate, note: `Biggest gate: ${gates.get(topGate.id)} tickets wait on it · ${nodeStatus(topGate, now).label}`, tone: nodeStatus(topGate, now).tone });
  }

  const merges = tickets
    .filter((t) => isCompletedStage(t.stage))
    .map((t) => Date.parse(t.lane_changed_at))
    .filter((at) => !Number.isNaN(at));
  let totalRetries = 0;
  let totalPasses = 0;
  for (const t of open) {
    totalRetries += retries(t).total;
    const a = t.attempts;
    if (a !== undefined) totalPasses += a.build + a.test + a.review + a.pr;
  }

  return {
    stuck,
    help: help.slice(0, INSIGHT_LIMIT + 2),
    progress: {
      working: open.filter((t) => nodeStatus(t, now).tone === "working" || (nodeStatus(t, now).tone === "windows" && hasLiveLease(t, now))).length,
      merged24h: merges.filter((at) => now - at <= DAY_MS).length,
      merged7d: merges.filter((at) => now - at <= MERGED_WINDOW_MS).length,
      lastMergeAt: merges.length === 0 ? null : Math.max(...merges),
      idle: open.filter((t) => nodeStatus(t, now).tone === "idle").length,
    },
    waste: {
      totalRetries,
      totalPasses,
      hotSpots: open
        .map((t) => ({ t, r: retries(t) }))
        .filter(({ r }) => r.total >= RETRY_WARM)
        .sort((a, b) => b.r.total - a.r.total || a.t.id - b.t.id)
        .slice(0, INSIGHT_LIMIT)
        .map(({ t, r }) => ({ ticket: t, note: `↻${r.total}: ${r.detail}`, tone: r.total >= RETRY_HOT ? "stuck" : "warm" })),
    },
  };
}

/** A repository's rough finish estimate. */
export interface LaneForecast {
  /** Merges per day over the last 7 days. */
  closeRate: number;
  /** Days to merge every open ticket at the current close rate; null with no merges. */
  throughputDays: number | null;
  /** Days for the longest dependency chain, one ticket after another at the median cycle time; null without one. */
  chainDays: number | null;
  /** The larger of the two: the binding constraint. */
  etaDays: number | null;
  bound: "throughput" | "dependencies" | "unknown";
}

/**
 * Two floors on when a repo's open work could all be merged: its backlog at
 * the recent close rate, and its longest dependency chain worked strictly in
 * series at the board's median cycle time. Whichever is larger binds; it is
 * an estimate to steer by, not a promise.
 */
export function forecastLane(lane: Pick<FlowLane, "open" | "merged" | "criticalPath">, cycleMedianHours: number | null): LaneForecast {
  const closeRate = lane.merged / 7;
  const throughputDays = closeRate > 0 ? lane.open / closeRate : null;
  const chainTickets = Math.max(lane.criticalPath.length, lane.open > 0 ? 1 : 0);
  const chainDays = cycleMedianHours !== null && cycleMedianHours > 0 && chainTickets > 0 ? (chainTickets * cycleMedianHours) / 24 : null;
  if (throughputDays === null && chainDays === null) return { closeRate, throughputDays, chainDays, etaDays: null, bound: "unknown" };
  if (throughputDays === null) return { closeRate, throughputDays, chainDays, etaDays: null, bound: "unknown" };
  const bound = chainDays !== null && chainDays > throughputDays ? "dependencies" : "throughput";
  return { closeRate, throughputDays, chainDays, etaDays: Math.max(throughputDays, chainDays ?? 0), bound };
}

/** "3.5 d", "18 h" or "6 wk" for a duration in days. */
export function formatDays(days: number): string {
  if (days < 1) return `${Math.max(1, Math.round(days * 24))} h`;
  if (days < 21) return `${days < 10 ? days.toFixed(1).replace(/\.0$/, "") : Math.round(days)} d`;
  return `${Math.round(days / 7)} wk`;
}


/** Which tickets stay lit while one is focused, and the chain behind it. */
export interface FlowFocus {
  id: number;
  upstream: Set<number>;
  downstream: Set<number>;
  path: number[];
}

/** The focus for a selected ticket (its upstream, downstream and critical path). */
export function focusOn(layout: FlowLayout, id: number): FlowFocus {
  return {
    id,
    upstream: upstreamOf(layout.graph, id),
    downstream: downstreamOf(layout.graph, id),
    path: criticalPath(layout.graph, id, layout.gates),
  };
}

/** Edges along a path, as "from>to" keys. */
export function pathEdgeKeys(path: readonly number[]): Set<string> {
  const keys = new Set<string>();
  for (let i = 0; i + 1 < path.length; i++) keys.add(`${path[i]}>${path[i + 1]}`);
  return keys;
}
