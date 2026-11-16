import { describe, expect, it } from "vitest";
import { teamFixture, ticketDetailFixture, TEAM_NOW } from "../components/team/teamFixture";
import {
  activityLines, formatTokens, hostLoad, humanizeDoing, linkPath, runnerEngine, runnerShortName, ticketClock, ticketRole, ticketWhere,
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

  it("turns build-server markers and raw signals into one readable line", () => {
    expect(humanizeDoing("build-server:testing:Windows x64 · awaiting Windows")).toBe("Windows x64 build testing");
    expect(humanizeDoing("repairing the release workflow")).toBe("Repairing the release workflow");
    expect(humanizeDoing("")).toBe("");
  });
});

describe("tickets in flight", () => {
  it("places a ticket on a host slot, in a host queue or with its runner", () => {
    const team = teamFixture();
    const [running, queued] = team.tickets;
    expect(ticketWhere(running!, team.build_hosts)).toBe("builder-1 slot 1 · testing");
    expect(ticketWhere(queued!, team.build_hosts)).toBe("builder-1 queue · 1st");
    expect(ticketWhere({ ...running!, build: null }, [])).toBe("codex_1");
    expect(ticketWhere({ ...queued!, build: null, waiting_on: [4] }, [])).toBe("waits on #4");
  });

  it("shows how long the build has run or waited", () => {
    const [running, queued] = teamFixture().tickets;
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

  it("summarizes host load", () => {
    const host = teamFixture().build_hosts[0]!;
    expect(hostLoad(host).text).toBe("1 of 2 slots busy");
    expect(hostLoad({ ...host, running: [] }).text).toBe("2 slots free");
    expect(hostLoad({ ...host, online: false }).text).toBe("offline");
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
