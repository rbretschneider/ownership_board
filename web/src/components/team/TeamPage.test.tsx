import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { BoardEvent } from "../../api/types";
import { teamFixture, ticketDetailFixture, TEAM_NOW } from "./teamFixture";
import { TeamPage } from "./TeamPage";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const stream = vi.hoisted(() => ({ onEvent: null as null | ((event: BoardEvent) => void) }));
vi.mock("../../hooks/useEventStream", () => ({
  useEventStream: (onEvent: (event: BoardEvent) => void) => {
    stream.onEvent = onEvent;
    return true;
  },
}));

let host: HTMLDivElement;
let root: Root;
let api: { team: ReturnType<typeof vi.fn>; ticket: ReturnType<typeof vi.fn> };

beforeEach(async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(TEAM_NOW);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  api = { team: vi.fn().mockResolvedValue(teamFixture()), ticket: vi.fn().mockResolvedValue(ticketDetailFixture()) };
  await act(async () => root.render(<ApiContext.Provider value={api as unknown as BoardApi}><TeamPage /></ApiContext.Provider>));
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.useRealTimers();
});

const runner = (name: string) => host.querySelector<HTMLButtonElement>(`[data-runner="${name}"]`)!;
const panel = () => host.querySelector<HTMLElement>('[role="dialog"]');
const press = async (key: string) => act(async () => { window.dispatchEvent(new KeyboardEvent("keydown", { key })); });

it("draws the org chart: manager, runners, tickets in flight and build hosts", () => {
  expect(host.querySelector('a[aria-current="page"]')?.textContent).toBe("Team");
  expect(host.textContent).toContain("Board manager");
  expect(host.textContent).toContain("$1.94");
  expect(host.textContent).toContain("Last review");
  expect(runner("@worker/7/codex_1").textContent).toContain("codex_1");
  expect(runner("@worker/7/codex_1").textContent).toContain("Working");
  expect(runner("@worker/7/claude_2").textContent).toContain("Idle");
  expect(host.querySelectorAll("[data-ticket]")).toHaveLength(2);
  expect(host.querySelector('[data-ticket="12"]')?.textContent).toContain("builder-1 slot 1 · testing");
  expect(host.textContent).toContain("1 of 2 slots busy");
  expect(host.textContent).toContain("Queue: #13 waiting 1h 06m");
});

it("lists CI runners as busy, idle or offline, and flags a stale sample", async () => {
  const row = (name: string) => host.querySelector(`[data-ci-runner="${name}"]`)?.textContent;
  expect(row("linux-1")).toContain("busy · linux, X64");
  expect(row("linux-2")).toContain("idle");
  expect(row("win-old")).toContain("offline · windows");
  expect(host.querySelector('[aria-label="CI runners"]')?.textContent).not.toContain("sampled");

  const stale = teamFixture();
  stale.ci_runners = { ...stale.ci_runners!, observed_at: new Date(TEAM_NOW - 40 * 60_000).toISOString() };
  api.team.mockResolvedValue(stale);
  await act(async () => {
    stream.onEvent?.({ type: "ticket_moved", ticket_id: 12 } as unknown as BoardEvent);
    await new Promise((resolve) => setTimeout(resolve, 400));
  });
  expect(host.querySelector('[aria-label="CI runners"]')?.textContent).toContain("sampled 40m ago");
});

it("hides the CI runners card when the server has no sample", async () => {
  const none = teamFixture();
  none.ci_runners = { observed_at: null, runners: [] };
  api.team.mockResolvedValue(none);
  await act(async () => {
    stream.onEvent?.({ type: "ticket_moved", ticket_id: 12 } as unknown as BoardEvent);
    await new Promise((resolve) => setTimeout(resolve, 400));
  });
  expect(host.querySelector('[aria-label="CI runners"]')).toBeNull();
  expect(host.textContent).toContain("builder-1");
});

it("links a hovered or focused runner to its ticket and clears the link on leave", async () => {
  expect(host.querySelector(".team-wire")).toBeNull();
  await act(async () => { runner("@worker/7/codex_1").dispatchEvent(new MouseEvent("mouseover", { bubbles: true })); });
  expect(host.querySelector(".team-wire__path")?.getAttribute("d")).toMatch(/^M/);
  expect(host.querySelector('[data-ticket="12"]')?.className).toContain("team-card--linked");
  expect(host.querySelector('[data-ticket="13"]')?.className).not.toContain("team-card--linked");
  await act(async () => { runner("@worker/7/codex_1").dispatchEvent(new MouseEvent("mouseout", { bubbles: true })); });
  expect(host.querySelector(".team-wire")).toBeNull();
  // An idle runner holds nothing, so focusing it draws no line.
  await act(async () => { runner("@worker/7/claude_2").focus(); });
  expect(host.querySelector(".team-wire")).toBeNull();
});

it("opens a runner's details with its ticket's activity, and closes with ×, Escape or a tap outside", async () => {
  await act(async () => runner("@worker/7/codex_1").click());
  expect(api.ticket).toHaveBeenCalledWith(12);
  expect(panel()?.getAttribute("aria-label")).toBe("codex_1");
  expect(panel()?.textContent).toContain("Recent activity");
  expect(panel()?.textContent).toContain("#12 Windows x64 build queued (gpt-test)");
  expect(host.querySelector(".team-wire")).not.toBeNull();

  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="Close details"]')!.click());
  expect(panel()).toBeNull();

  await act(async () => runner("@worker/7/codex_1").click());
  await press("Escape");
  expect(panel()).toBeNull();

  await act(async () => runner("@worker/7/claude_2").click());
  expect(panel()?.textContent).toContain("No ticket held");
  await act(async () => host.querySelector<HTMLElement>('[data-testid="team-backdrop"]')!.click());
  expect(panel()).toBeNull();
});

it("opens the manager's history log", async () => {
  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="Board manager: open history"]')!.click());
  expect(panel()?.textContent).toContain("History log");
  expect(panel()?.textContent).toContain("Held #13 for a scope split.");
  expect(panel()?.textContent).toContain("$0.16 · sonnet");
  await press("Escape");
  expect(panel()).toBeNull();
});

it("refreshes from the board's event stream", async () => {
  const next = teamFixture();
  next.runners = next.runners.slice(1);
  api.team.mockResolvedValue(next);
  await act(async () => {
    stream.onEvent?.({ type: "ticket_moved", ticket_id: 12 } as unknown as BoardEvent);
    // The refetch is debounced, like the board's.
    await new Promise((resolve) => setTimeout(resolve, 400));
  });
  expect(api.team).toHaveBeenCalledTimes(2);
  expect(runner("@worker/7/codex_1")).toBeNull();
});
