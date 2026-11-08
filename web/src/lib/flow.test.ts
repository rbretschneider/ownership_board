import { describe, expect, it } from "vitest";
import {
  buildGraph,
  criticalPath,
  downstreamOf,
  flowInsights,
  flowStage,
  focusOn,
  forecastLane,
  formatDays,
  gateCounts,
  laneOptions,
  layoutFlow,
  nodeStatus,
  retries,
  stageColumnKey,
  upstreamOf,
} from "./flow";
import { LIVE_LEASE, liveLikeBoard, NOW, pausedOnWindows, repo, ticket } from "./flowFixture";

describe("flowStage", () => {
  it("splits build by the work-model signal, like the card stepper", () => {
    expect(flowStage(ticket(1, { stage: "build", work_model: "m" }))).toBe("build");
    expect(flowStage(ticket(1, { stage: "build", work_model: "m · build-server:queued:Windows x64 · awaiting Windows" }))).toBe("windows");
    expect(flowStage(ticket(1, { stage: "build", work_model: "m · adversarial review" }))).toBe("review");
    expect(flowStage(ticket(1, { stage: "spec_review" }))).toBe("shaping");
    expect(flowStage(ticket(1, { stage: "design_review" }))).toBe("design");
    expect(flowStage(ticket(1, { stage: "pr" }))).toBe("pr");
    expect(flowStage(ticket(1, { stage: "done" }))).toBe("merged");
  });

  it("folds how work is done into one In progress column unless stages are shown", () => {
    const windows = ticket(1, { stage: "build", work_model: "m · build-server:queued:Windows x64 · awaiting Windows" });
    expect(stageColumnKey(windows, false)).toBe("doing");
    expect(stageColumnKey(ticket(1, { stage: "design" }), false)).toBe("doing");
    expect(stageColumnKey(ticket(1, { stage: "refine" }), false)).toBe("next");
    expect(stageColumnKey(windows, true)).toBe("windows");
  });
});

describe("dependency graph", () => {
  it("counts hand-offs before a ticket can start", () => {
    const graph = buildGraph([ticket(1), ticket(2, { waiting_on: [1] }), ticket(3, { waiting_on: [2, 1] })]);
    expect(graph.depth.get(1)).toBe(0);
    expect(graph.depth.get(2)).toBe(1);
    expect(graph.depth.get(3)).toBe(2);
  });

  it("counts a dependency missing from the board as one hand-off", () => {
    const graph = buildGraph([ticket(5, { waiting_on: [999] })]);
    expect(graph.depth.get(5)).toBe(1);
    expect(graph.deps.get(5)).toEqual([]);
  });

  it("cuts a cycle instead of looping", () => {
    const graph = buildGraph([ticket(1, { waiting_on: [2] }), ticket(2, { waiting_on: [1] })]);
    expect(graph.depth.get(1)).toBeGreaterThan(0);
    expect(Number.isFinite(graph.depth.get(2))).toBe(true);
    expect(criticalPath(graph, 1)).toEqual([1, 2]);
  });

  it("finds what a ticket gates and the chain behind a milestone", () => {
    const { tickets } = liveLikeBoard();
    const graph = buildGraph(tickets);
    const gates = gateCounts(graph);
    expect(gates.get(1)).toBe(31);
    expect(downstreamOf(graph, 1).has(40)).toBe(true);
    expect(upstreamOf(graph, 40).size).toBe(32);
    const path = criticalPath(graph, 40, gates);
    expect(path[0]).toBe(40);
    expect(path).toHaveLength(12);
    expect(path[path.length - 1]).toBe(1);
  });
});

describe("layoutFlow", () => {
  it("runs every dependency line rightwards, from a ticket to the work it waits on", () => {
    const { tickets, repos } = liveLikeBoard();
    const layout = layoutFlow(tickets, repos, NOW);
    expect(layout.edges.length).toBeGreaterThan(30);
    for (const { from, to } of layout.edges) {
      expect(layout.nodes.get(to)?.x).toBeGreaterThan(layout.nodes.get(from)?.x ?? Infinity);
    }
  });

  it("puts the deepest queue furthest left and collapses empty columns", () => {
    const { tickets, repos } = liveLikeBoard();
    const layout = layoutFlow(tickets, repos, NOW);
    expect(layout.columns.map((c) => c.key).slice(0, 2)).toEqual(["queue-11", "queue-10"]);
    expect(layout.columns.map((c) => c.key).slice(-4)).toEqual(["next", "doing", "pr", "merged"]);
    expect(layout.columns.find((c) => c.key === "pr")?.isCollapsed).toBe(true);
    expect(layout.nodes.get(40)?.columnKey).toBe("queue-11");
    expect(layout.nodes.get(1)?.columnKey).toBe("doing");
  });

  it("shows only repos with open work, filtered, with a few recent merges each", () => {
    const { tickets, repos } = liveLikeBoard();
    const all = layoutFlow(tickets, repos, NOW);
    expect(all.lanes.map((l) => l.name)).toEqual(["alpha", "beta"]);
    expect([...all.nodes.values()].filter((n) => n.columnKey === "merged" && n.laneKey === "1")).toHaveLength(3);
    const betaOnly = layoutFlow(tickets, repos, NOW, { laneKeys: new Set(["2"]) });
    expect(betaOnly.lanes.map((l) => l.name)).toEqual(["beta"]);
    expect(betaOnly.columns[0]?.key).toBe("queue-2");
    expect(laneOptions(tickets, repos).map((o) => [o.name, o.open])).toEqual([["alpha", 33], ["beta", 3]]);
  });

  it("keeps the owner's board order within a column", () => {
    const tickets = [
      ticket(9, { lane: "in_progress", stage: "build" }),
      ticket(3, { lane: "in_progress", stage: "build" }),
      ticket(5, { lane: "in_progress", stage: "build" }),
    ];
    const layout = layoutFlow(tickets, [repo(1, "alpha")], NOW);
    const ys = [9, 3, 5].map((id) => layout.nodes.get(id)?.y ?? 0);
    expect(ys).toEqual([...ys].sort((a, b) => a - b));
  });

  it("names each lane's critical path to its deepest ticket", () => {
    const { tickets, repos } = liveLikeBoard();
    const alpha = layoutFlow(tickets, repos, NOW).lanes[0];
    expect(alpha?.milestoneId).toBe(40);
    expect(alpha?.criticalPath.at(-1)).toBe(1);
  });

  it("lays out ~90 tickets with 10-deep chains quickly", () => {
    const { tickets, repos } = liveLikeBoard();
    const start = performance.now();
    for (let i = 0; i < 10; i++) layoutFlow(tickets, repos, NOW, { isDetailed: i % 2 === 0 });
    expect((performance.now() - start) / 10).toBeLessThan(50);
  });

  it("focuses a milestone on its upstream, downstream and chain", () => {
    const { tickets, repos } = liveLikeBoard();
    const focus = focusOn(layoutFlow(tickets, repos, NOW), 10);
    expect(focus.upstream).toEqual(new Set([1]));
    expect(focus.downstream.has(40)).toBe(true);
    expect(focus.path).toEqual([10, 1]);
  });
});

describe("node status and retries", () => {
  it("reads a paused Windows build as stuck and counts its loops", () => {
    const t = pausedOnWindows(1);
    expect(nodeStatus(t, NOW).tone).toBe("stuck");
    expect(retries(t)).toEqual({ total: 25, detail: "25 failed Windows runs" });
  });

  it("separates working, waiting, owner and idle", () => {
    expect(nodeStatus(ticket(1, { stage: "build", lease: LIVE_LEASE }), NOW).tone).toBe("working");
    expect(nodeStatus(ticket(1, { waiting_on: [2] }), NOW).tone).toBe("waiting");
    expect(nodeStatus(ticket(1, { needs_owner: true, owner: "human", open_questions: 2 }), NOW).tone).toBe("owner");
    expect(nodeStatus(ticket(1, { stage: "build", lane: "in_progress" }), NOW).tone).toBe("idle");
  });
});

describe("flowInsights", () => {
  it("answers stuck, unblock, moving and waste for the live-like board", () => {
    const { tickets } = liveLikeBoard();
    const graph = buildGraph(tickets);
    const insights = flowInsights(tickets, gateCounts(graph), NOW);
    expect(insights.stuck[0]?.label).toBe("Windows");
    expect(insights.stuck[0]?.tickets[0]?.ticket.id).toBe(1);
    expect(insights.help[0]?.ticket.id).toBe(1);
    expect(insights.help[0]?.note).toContain("gates 31");
    expect(insights.progress.working).toBe(2);
    // Merges every 3 h from now: 0 h through 24 h inclusive.
    expect(insights.progress.merged24h).toBe(9);
    expect(insights.waste.totalRetries).toBe(25);
    expect(insights.waste.hotSpots[0]?.tone).toBe("stuck");
  });

  it("puts the owner's own questions first", () => {
    const tickets = [pausedOnWindows(1), ticket(2, { needs_owner: true, owner: "human", open_questions: 2 })];
    const help = flowInsights(tickets, gateCounts(buildGraph(tickets)), NOW).help;
    expect(help.map((h) => [h.ticket.id, h.note])).toEqual([[2, "Answer 2 questions"], [1, "Paused on the Windows PC; resume or rescope"]]);
  });
});

describe("forecastLane", () => {
  it("is bound by close rate when the backlog is the long pole", () => {
    const f = forecastLane({ open: 21, merged: 14, criticalPath: [1, 2] }, 4.5);
    expect(f.closeRate).toBe(2);
    expect(f.throughputDays).toBeCloseTo(10.5);
    expect(f.bound).toBe("throughput");
    expect(f.etaDays).toBeCloseTo(10.5);
  });

  it("is bound by dependencies when the chain is the long pole", () => {
    const f = forecastLane({ open: 12, merged: 14, criticalPath: new Array(12).fill(0) }, 24);
    expect(f.bound).toBe("dependencies");
    expect(f.etaDays).toBe(12);
  });

  it("gives no estimate without recent merges", () => {
    expect(forecastLane({ open: 4, merged: 0, criticalPath: [] }, 4).etaDays).toBeNull();
  });

  it("formats durations", () => {
    expect(formatDays(0.2)).toBe("5 h");
    expect(formatDays(2)).toBe("2 d");
    expect(formatDays(3.46)).toBe("3.5 d");
    expect(formatDays(42)).toBe("6 wk");
  });
});

it("groups deployed work as completed and excludes it from open dependencies", () => {
 const done = ticket(1, {stage: "deployed", lane: "deployed", lane_changed_at: new Date(NOW).toISOString()});
 const open = ticket(2, {waiting_on: [1]});
 expect(flowStage(done)).toBe("merged");
 expect(nodeStatus(done, NOW)).toEqual({tone: "done", label: "Deployed"});
 expect(buildGraph([done, open]).deps.get(2)).toEqual([]);
 expect(laneOptions([done], [])).toEqual([]);
 const layout = layoutFlow([done, open], [], NOW);
 expect(layout.nodes.get(1)?.columnKey).toBe("merged");
 expect(layout.lanes[0]?.open).toBe(1);
 expect(flowInsights([done], new Map(), NOW).progress).toMatchObject({idle: 0, working: 0, merged24h: 1});
});
