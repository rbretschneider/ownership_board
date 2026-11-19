import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { Team } from "../../api/types";
import { teamFixture, ticketDetailFixture, TEAM_NOW } from "./teamFixture";
import { TeamPage } from "./TeamPage";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

vi.mock("../../hooks/useEventStream", () => ({ useEventStream: () => true }));

let host: HTMLDivElement;
let root: Root;

async function mount(team: Team) {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(TEAM_NOW);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  const api = { team: vi.fn().mockResolvedValue(team), ticket: vi.fn().mockResolvedValue(ticketDetailFixture()) };
  await act(async () => root.render(<ApiContext.Provider value={api as unknown as BoardApi}><TeamPage /></ApiContext.Provider>));
}

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.useRealTimers();
});

function teamWith(lastSeenSecondsAgo: number, inFlight: number, cap: number): Team {
  const team = teamFixture();
  team.claims = { in_flight: inFlight, cap };
  team.idle_poll_seconds = 30;
  team.runners = team.runners.map((r) => r.state === "idle" ? { ...r, last_seen: new Date(TEAM_NOW - lastSeenSecondsAgo * 1000).toISOString() } : r);
  return team;
}

const idleCard = () => host.querySelector('[data-runner="@worker/7/claude_2"]')!;
const workingCard = () => host.querySelector('[data-runner="@worker/7/codex_1"]')!;

it("counts an idle runner down to its next claim and says the in-flight cap is why it is idle", async () => {
  await mount(teamWith(12, 7, 7));
  const timer = idleCard().querySelector('[role="timer"]')!;
  expect(timer.textContent).toBe("next check in 18s");
  expect(timer.querySelector("svg circle.team-poll__arc")).not.toBeNull();
  expect(idleCard().textContent).toContain("In-flight cap full: 7 of 7 tickets being worked");
  // A working runner holds a ticket and shows no countdown.
  expect(workingCard().querySelector('[role="timer"]')).toBeNull();
});

it("reads checking… once the wait has run out, and says nothing about the cap when there is room", async () => {
  await mount(teamWith(45, 3, 7));
  expect(idleCard().querySelector('[role="timer"]')!.textContent).toBe("checking…");
  expect(idleCard().textContent).not.toContain("In-flight cap");
});
