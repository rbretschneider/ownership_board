import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { RunnerName, Team } from "../../api/types";
import { RunnerNamesContext } from "../../hooks/useRunnerNames";
import { EMPTY_RUNNER_DIRECTORY, runnerDirectory, type RunnerDirectory } from "../../lib/runnerNames";
import { teamFixture, ticketDetailFixture, TEAM_NOW } from "./teamFixture";
import { TeamPage } from "./TeamPage";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

vi.mock("../../hooks/useEventStream", () => ({ useEventStream: () => true }));

let host: HTMLDivElement;
let root: Root;

const named = (worker: string, family: string, displayName: string): RunnerName =>
  ({ worker, family, position: 1, display_name: displayName, assigned_at: null });

async function renderTeam(directory: RunnerDirectory, team: Team = teamFixture()) {
  const api = { team: vi.fn().mockResolvedValue(team), ticket: vi.fn().mockResolvedValue(ticketDetailFixture()) };
  await act(async () => root.render(
    <ApiContext.Provider value={api as unknown as BoardApi}>
      <RunnerNamesContext.Provider value={{ directory, refresh: () => {} }}><TeamPage /></RunnerNamesContext.Provider>
    </ApiContext.Provider>,
  ));
}

const runnerName = (identity: string) => host.querySelector<HTMLElement>(`[data-runner="${identity}"] .team-runner__name`)!;

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(TEAM_NOW);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.useRealTimers();
});

it("shows each runner as \"Name (worker)\" with its raw identity in the tooltip", async () => {
  await renderTeam(runnerDirectory([named("codex_1", "codex", "Ray"), named("claude_2", "claude", "Dwight")]));
  expect(runnerName("@worker/7/codex_1").textContent).toBe("Ray (codex_1)");
  expect(runnerName("@worker/7/codex_1").title).toBe("@worker/7/codex_1");
  expect(runnerName("@worker/7/claude_2").textContent).toBe("Dwight (claude_2)");
  expect(host.querySelector(".runner-name__family")).toBeNull();
  await act(async () => host.querySelector<HTMLButtonElement>('[data-runner="@worker/7/codex_1"]')!.click());
  expect(host.querySelector('[role="dialog"]')?.textContent).toContain("Ray (codex_1)");
});

it("badges the family when two families have a runner of the same name", async () => {
  await renderTeam(runnerDirectory([named("codex_1", "codex", "Michael"), named("claude_2", "claude", "Michael")]));
  expect(runnerName("@worker/7/codex_1").querySelector(".runner-name__family")?.textContent).toBe("codex");
  expect(runnerName("@worker/7/claude_2").querySelector(".runner-name__family")?.textContent).toBe("claude");
});

it("uses the name the team payload carries until the directory has it, and the raw id without one", async () => {
  const team = teamFixture();
  team.runners = team.runners.map((r) => (r.name === "@worker/7/codex_1" ? { ...r, display_name: "Ray", family: "codex" } : r));
  await renderTeam(EMPTY_RUNNER_DIRECTORY, team);
  expect(runnerName("@worker/7/codex_1").textContent).toBe("Ray (codex_1)");
  expect(runnerName("@worker/7/claude_2").textContent).toBe("claude_2");
});
