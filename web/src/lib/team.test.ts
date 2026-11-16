import { describe, expect, it } from "vitest";
import { teamFixture, ticketDetailFixture, TEAM_NOW } from "../components/team/teamFixture";
import {
  activityLines, ciRunnerStatus, formatTokens, historyEntry, hostStatus, humanizeDoing, JOB_RUNNING_TOO_LONG_MS, linkPath, managerTargets, runnerEngine, runnerPool, runnerShortName, slotStatus,
  ticketActivity, ticketClock, ticketRole,
} from "./team";

describe("runner labels", () => {
  it("shortens worker identities and names the provider", () => {
    expect(runnerShortName("@worker/7/codex_1")).toBe("codex_1");
    expect(runnerShortName("loop")).toBe("loop");
    expect(runnerEngine({ name: "@worker/7/codex_1", model: "gpt-test" })).toBe("Codex · gpt-test");
    expect(runnerEngine({ name: "@worker/7/claude_1", model: "opus-test [escalated:3]" })).toBe("Claude · opus-test · escalated");
    expect(runnerEngine({ name: "@worker/7/claude_1", model: "" })).toBe("Claude");
    expect(runnerEngine({ name: "loop", model: "" })).toBe("Model not reported");
  });

  it("takes the provider from the runner's pool and says when it escalated to the other family", () => {
    expect(runnerEngine({ name: "@worker/7/codex_1", model: "opus [escalated:2]" })).toBe("Codex · opus · escalated to Claude");
    expect(runnerEngine({ name: "@worker/7/claude_1", model: "gpt-test" })).toBe("Claude · gpt-test · escalated to Codex");
    expect(runnerEngine({ name: "@worker/7/codex_2", model: "" })).toBe("Codex");
  });

  it("turns build-server markers and raw signals into one readable line", () => {
    expect(humanizeDoing("build-server:testing:Windows x64 · awaiting Windows")).toBe("Windows x64 build testing");
    expect(humanizeDoing("repairing the release workflow")).toBe("Repairing the release workflow");
    expect(humanizeDoing("")).toBe("");
  });
});

describe("tickets in flight", () => {
  it("says what each ticket is doing or waiting on", () => {
    const team = teamFixture();
    const byId = (id: number) => team.tickets.find((t) => t.id === id)!;
    const of = (id: number) => ticketActivity(byId(id), team.build_hosts, TEAM_NOW);
    expect(of(12)).toEqual({ group: "active", text: "Testing on builder-1 slot 1", tone: "accent" });
    expect(of(16)).toMatchObject({ group: "active", text: "PR open, CI 7/8" });
    expect(of(13)).toMatchObject({ group: "waiting", text: "1st in builder-1 queue" });
    expect(of(15)).toMatchObject({ group: "waiting", text: "Waiting on #12" });
    expect(of(14)).toMatchObject({ group: "blocked", text: "Blocked: Needs the owner: signing certificate expired" });
    const leased = { ...byId(12), lease: { holder: "@worker/7/codex_1", expires_at: new Date(TEAM_NOW + 60_000).toISOString() }, work_model: "gpt-test · repairing the workflow" };
    expect(ticketActivity(leased, [], TEAM_NOW)).toMatchObject({ group: "active", text: "codex_1: Repairing the workflow" });
    const green = { ...byId(16), pr: { ...byId(16).pr!, verdict: "green_owner", checks_done: 8 } };
    expect(ticketActivity(green, [], TEAM_NOW)).toMatchObject({ group: "waiting", text: "PR green, waiting to merge" });
    expect(ticketActivity({ ...byId(15), waiting_on: [] }, [], TEAM_NOW).text).toBe("Ready for a runner");
  });

  it("names a runner's pool, and an escalation to the other family", () => {
    expect(runnerPool({ name: "@worker/7/codex_1", model: "opus [escalated:2]" })).toBe("Codex runner · escalated to Claude");
    expect(runnerPool({ name: "@worker/7/claude_1", model: "opus" })).toBe("Claude runner");
    expect(runnerPool({ name: "loop", model: "" })).toBe("Agent runner");
  });

  it("splits a structured history entry into headline and markdown, and falls back for older ones", () => {
    const structured = historyEntry({ summary: "Board review\nMANAGER SUMMARY\nHEADLINE: Merged #12.\n\n### Acted\n- #12 merged.", headline: "Merged #12.", structured: true });
    expect(structured).toEqual({ headline: "Merged #12.", body: "### Acted\n- #12 merged.", isMarkdown: true });
    const legacy = historyEntry({ summary: "MANAGER SUMMARY\nRepeated #64 directions. why: x | action: y | expect: z", headline: "Repeated #64 directions." });
    expect(legacy).toEqual({ headline: "Repeated #64 directions.", body: "why: x\naction: y\nexpect: z", isMarkdown: false });
    expect(historyEntry({ summary: "Checked the host. All fine." }).headline).toBe("Checked the host.");
  });

  it("points the working manager at its tickets, their runners and a named host", () => {
    const team = teamFixture();
    expect(managerTargets(team)).toEqual([]);
    team.manager.activity = { state: "working", kind: "flag", ticket_ids: [12, 99, 13], host: "builder-1", started_at: null };
    expect(managerTargets(team)).toEqual([
      { kind: "ticket", key: "12" }, { kind: "runner", key: "@worker/7/codex_1" }, { kind: "ticket", key: "13" }, { kind: "host", key: "builder-1" },
    ]);
    team.manager.activity = { ...team.manager.activity, host: "unknown-host", ticket_ids: [] };
    expect(managerTargets(team)).toEqual([]);
  });

  it("shows how long the build has run or waited", () => {
    const team = teamFixture();
    const running = team.tickets.find((t) => t.id === 12);
    const queued = team.tickets.find((t) => t.id === 13);
    expect(ticketClock(running!, TEAM_NOW)).toBe("1h 36m on host");
    expect(ticketClock(queued!, TEAM_NOW)).toBe("queued 1h 06m");
    expect(ticketClock({ ...queued!, build: null }, TEAM_NOW)).toBe("—");
  });

  it("names roles and token counts", () => {
    expect(ticketRole("build").label).toBe("Engineer");
    expect(ticketRole("pr").label).toBe("Review");
    expect(formatTokens(1_900_000)).toBe("1.9M tok");
    expect(formatTokens(820_400)).toBe("820k tok");
    expect(formatTokens(undefined)).toBe("0 tok");
  });

  it("puts hosts, slots and CI runners on one idle / busy / problem scale", () => {
    const now = TEAM_NOW;
    const host = teamFixture().build_hosts[0]!;
    expect(hostStatus(host, now)).toEqual({ tone: "busy", text: "2 of 2 building" });
    expect(hostStatus({ ...host, running: host.running.slice(0, 1) }, now).text).toBe("1 of 2 building");
    expect(hostStatus({ ...host, running: [] }, now)).toEqual({ tone: "idle", text: "idle" });
    expect(hostStatus({ ...host, online: false }, now)).toEqual({ tone: "problem", text: "offline" });

    expect(slotStatus(host, host.running[1], now)).toEqual({ tone: "busy", text: "#17 · building · 40m" });
    expect(slotStatus(host, undefined, now)).toEqual({ tone: "idle", text: "idle" });
    expect(slotStatus({ ...host, online: false }, host.running[1], now)).toEqual({ tone: "problem", text: "#17 · host offline" });
    // Past the manager's stuck-job limit, a running job is a problem.
    const stuck = { ...host.running[1]!, started_at: new Date(now - JOB_RUNNING_TOO_LONG_MS - 60_000).toISOString() };
    expect(slotStatus(host, stuck, now).tone).toBe("problem");
    expect(slotStatus(host, stuck, now).text).toMatch(/· stuck$/);
    expect(hostStatus({ ...host, running: [stuck] }, now)).toEqual({ tone: "problem", text: "1 stuck" });

    const ci = teamFixture().ci_runners!.runners;
    expect(ci.map((r) => ciRunnerStatus(r).tone)).toEqual(["busy", "idle", "problem"]);
  });
});

describe("activity", () => {
  it("lists a ticket's newest history first", () => {
    const lines = activityLines(ticketDetailFixture().timeline, 12);
    expect(lines.map((l) => l.tag)).toEqual(["comment", "model", "claim"]);
    expect(lines[0]?.text).toBe("Windows verification queued for commit abc1234");
    expect(lines[1]?.text).toBe("#12 Windows x64 build queued (gpt-test)");
  });
});

describe("linkPath", () => {
  const container = { left: 100, top: 50, width: 800, height: 600 };

  it("curves from the runner's bottom to the ticket's top in container coordinates", () => {
    const line = linkPath(container, { left: 120, top: 100, width: 200, height: 80 }, { left: 500, top: 300, width: 100, height: 60 });
    expect(line.x2).toBe(450);
    expect(line.y2).toBe(250);
    expect(line.d).toBe("M120 130 C 120 190, 450 190, 450 250");
  });

  it("undoes the container's scale", () => {
    const scaled = { left: 100, top: 50, width: 400, height: 300 };
    const line = linkPath(scaled, { left: 110, top: 75, width: 100, height: 40 }, { left: 300, top: 175, width: 50, height: 30 }, 0.5);
    expect(line.x2).toBe(450);
    expect(line.y2).toBe(250);
    expect(line.d.startsWith("M120 130 ")).toBe(true);
  });

  it("runs upward when the ticket sits above the runner", () => {
    const line = linkPath(container, { left: 120, top: 400, width: 200, height: 80 }, { left: 500, top: 100, width: 100, height: 60 });
    expect(line.y2).toBe(110);
    expect(line.d.startsWith("M120 350 ")).toBe(true);
  });
});
