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

it("draws the org chart: manager, runners with their tickets, and build hosts", () => {
  expect(host.querySelector('a[aria-current="page"]')?.textContent).toBe("Team");
  expect(host.textContent).toContain("Board manager");
  expect(host.textContent).toContain("$1.94");
  expect(runner("@worker/7/codex_1").textContent).toContain("codex_1");
  expect(runner("@worker/7/codex_1").textContent).toContain("Working");
  expect(runner("@worker/7/claude_2").textContent).toContain("Idle");
  expect(host.querySelector('[data-ticket="12"]')?.textContent).toContain("Testing on builder-1 slot 1");
  expect(host.textContent).toContain("2 of 2 slots busy");
  expect(host.textContent).toContain("Queue: #13 waiting 1h 06m");
});

it("nests each ticket under the runner working it, and a ticket no runner holds in its host slot", () => {
  expect(host.textContent).not.toContain("Tickets in flight");
  const branch = runner("@worker/7/codex_1").closest(".team-branch")!;
  const child = branch.querySelector('[data-ticket="12"]')!;
  expect(child.textContent).toContain("Testing on builder-1 slot 1");
  // The Board card's own signals: the stage stepper with attempt counts, and the token badge.
  expect(child.querySelector(".stepper")).not.toBeNull();
  expect(child.querySelector(".stepper__count")?.textContent).toBe("3");
  expect(child.querySelector(".badge--usage")?.textContent).toContain("1.9M tok");
  expect(runner("@worker/7/claude_2").closest(".team-branch")!.querySelector("[data-ticket]")).toBeNull();
  const slot = host.querySelector('[data-host="builder-1"] [data-ticket="17"]');
  expect(slot?.closest(".team-slot")).not.toBeNull();
  // Each ticket appears once: #12 under its runner, not again in its slot; #13 is only queued.
  expect(host.querySelectorAll('[data-ticket="12"]')).toHaveLength(1);
  expect(host.querySelector('[data-ticket="13"]')).toBeNull();
});

it("shows the manager's headline, not its raw summary, and no feed bar or profile chips", () => {
  const card = host.querySelector('[aria-label="Board manager: open history"]')!;
  expect(card.textContent).toContain("Directed #12 to fix its brand-scan test.");
  expect(card.textContent).not.toContain("MANAGER SUMMARY");
  expect(card.textContent).not.toContain("cycling");
  expect(host.querySelector(".team-feed")).toBeNull();
  expect(host.textContent).not.toContain("profile not reported");
  // A hint is not a link, so it must not wear the link style.
  expect(card.querySelector(".team-hint")?.textContent).toBe("Tap for history log");
});

it("draws the manager's lines only while it works, to each ticket, its runner and the host", async () => {
  expect(host.querySelector('[data-testid="manager-lines"]')).toBeNull();
  const working = teamFixture();
  working.manager.activity = { state: "working", kind: "flag", ticket_ids: [12], host: "builder-1", started_at: null };
  api.team.mockResolvedValue(working);
  await act(async () => {
    stream.onEvent?.({ type: "ticket_moved", ticket_id: 12 } as unknown as BoardEvent);
    await new Promise((resolve) => setTimeout(resolve, 400));
  });
  const targets = [...host.querySelectorAll('[data-testid="manager-lines"] [data-target]')].map((g) => g.getAttribute("data-target"));
  expect(targets).toEqual(["ticket:12", "runner:@worker/7/codex_1", "host:builder-1"]);
  expect(host.querySelector('[aria-label="Board manager: open history"]')?.textContent).toContain("Working on #12, builder-1");

  api.team.mockResolvedValue(teamFixture());
  await act(async () => {
    stream.onEvent?.({ type: "ticket_moved", ticket_id: 12 } as unknown as BoardEvent);
    await new Promise((resolve) => setTimeout(resolve, 400));
  });
  expect(host.querySelector('[data-testid="manager-lines"]')).toBeNull();
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

it("nests every ticket a runner holds or waits on under it", async () => {
  const two = teamFixture();
  // #13 waits on a host queue after codex_1 released it: it is codex_1's too.
  two.tickets = two.tickets.map((t) => (t.id === 13 ? { ...t, holder: "@worker/7/codex_1" } : t));
  api.team.mockResolvedValue(two);
  await act(async () => {
    stream.onEvent?.({ type: "ticket_moved", ticket_id: 13 } as unknown as BoardEvent);
    await new Promise((resolve) => setTimeout(resolve, 400));
  });
  const branch = runner("@worker/7/codex_1").closest(".team-branch")!;
  expect([...branch.querySelectorAll("[data-ticket]")].map((el) => el.getAttribute("data-ticket"))).toEqual(["12", "13"]);
  expect(branch.querySelector('[data-ticket="13"]')?.textContent).toContain("1st in builder-1 queue");
});

it("highlights a hovered or focused runner's branch without drawing a line", async () => {
  await act(async () => { runner("@worker/7/codex_1").dispatchEvent(new MouseEvent("mouseover", { bubbles: true })); });
  expect(runner("@worker/7/codex_1").className).toContain("team-card--active");
  expect(host.querySelector('[data-ticket="12"]')?.className).toContain("team-card--linked");
  expect(host.querySelector(".team-wire")).toBeNull();
  await act(async () => { runner("@worker/7/codex_1").dispatchEvent(new MouseEvent("mouseout", { bubbles: true })); });
  expect(host.querySelector('[data-ticket="12"]')?.className).not.toContain("team-card--linked");
  await act(async () => { runner("@worker/7/claude_2").focus(); });
  expect(runner("@worker/7/claude_2").className).toContain("team-card--active");
});

it("opens a runner's details with its ticket's activity, and closes with ×, Escape or a tap outside", async () => {
  await act(async () => runner("@worker/7/codex_1").click());
  expect(api.ticket).toHaveBeenCalledWith(12);
  expect(panel()?.getAttribute("aria-label")).toBe("codex_1");
  expect(panel()?.textContent).toContain("Recent activity");
  expect(panel()?.textContent).toContain("#12 Windows x64 build queued (gpt-test)");
  expect(panel()?.closest(".team-modal")).not.toBeNull();
  expect(document.activeElement?.getAttribute("aria-label")).toBe("Close details");

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

it("shows the manager's history as headlines, the newest expanded as markdown and older ones behind a toggle", async () => {
  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="Board manager: open history"]')!.click());
  const items = [...panel()!.querySelectorAll(".team-history__item")];
  expect(items).toHaveLength(2);
  const [newest, older] = items;
  expect(newest!.querySelector(".team-history__headline")?.textContent).toBe("Directed #12 to fix its brand-scan test.");
  expect(newest!.querySelector(".team-chip")?.textContent).toBe("$0.10 · sonnet");
  // happy-dom's sanitizer drops the first element's tag, so check the text and a later heading.
  expect(newest!.querySelector(".markdown")?.textContent).toContain("#12 Release pipeline");
  expect([...newest!.querySelectorAll(".markdown h3")].map((h) => h.textContent)).toContain("Owner");
  expect(newest!.querySelector(".markdown strong")?.textContent).toBe("Found:");
  expect(newest!.textContent).not.toContain("MANAGER SUMMARY");
  expect(newest!.textContent).not.toContain("HEADLINE:");

  expect(older!.querySelector(".team-history__headline")?.textContent).toBe("Held #13 for a scope split.");
  expect(older!.querySelector(".team-history__body")).toBeNull();
  const toggle = older!.querySelector<HTMLButtonElement>(".team-history__toggle")!;
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  await act(async () => toggle.click());
  // A legacy summary is plain text, its " | " parts on their own lines.
  expect(older!.querySelector(".team-history__plain")?.textContent).toBe("why: too large\naction: parked\nexpect: a follow-up ticket");
  await press("Escape");
  expect(panel()).toBeNull();
});

it("keeps focus inside the modal", async () => {
  await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="Board manager: open history"]')!.click());
  const dialog = panel()!;
  const focusable = [...dialog.querySelectorAll<HTMLElement>("button")];
  const last = focusable[focusable.length - 1]!;
  last.focus();
  await act(async () => { dialog.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true })); });
  expect(document.activeElement?.getAttribute("aria-label")).toBe("Close details");
  await act(async () => { dialog.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", shiftKey: true, bubbles: true })); });
  expect(document.activeElement).toBe(last);
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
