import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import { CurrentUserContext } from "../../hooks/useCurrentUser";
import { deploymentRepo, deploymentTicket, deploymentUser } from "../ticket/deploymentFixture";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import type { Board, BoardEvent, PRStatusSnapshot } from "../../api/types";
import type { DropTarget } from "../../lib/dropTargets";
import { EMPTY_RUNNERS } from "../../lib/runners";
import { lanes, prTicket } from "./boardFixture";
import { BoardScreen } from "./BoardScreen";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

const mock = vi.hoisted(() => ({ move: vi.fn().mockResolvedValue(null), drop: null as null | ((id: number, target: DropTarget) => void), onEvent: null as null | ((event: BoardEvent) => void), onResync: null as null | (() => void) }));
const now = Date.parse("2026-11-08T12:00:00Z");
const snapshot: PRStatusSnapshot = {observed_at: new Date(now).toISOString(), prs: [{
 ticket_id: 2, url: "https://github.com/acme/app/pull/2", verdict: "green_owner", reason: "", may_merge: false, acts: false,
 gl_waived: false, draft: false, mergeable: "MERGEABLE", review: "", head: "abc123", unresolved: false, manual_check: false, checks: [],
}]};
const baseBoard: Board = {lanes, stages: [], repos: [], tickets: [prTicket, {...prTicket, id: 2, title: "Green change", needs_owner: false, pr_url: snapshot.prs[0]!.url}, {...prTicket, id: 3, title: "Other work", needs_owner: false, stage: "build"}, {...prTicket, id: 4, lane: "todo", stage: "ready", needs_owner: false}]};
vi.mock("../../hooks/useMediaQuery", () => ({useMediaQuery: () => true}));
vi.mock("../../hooks/useEventStream", () => ({useEventStream: (onEvent: (event: BoardEvent) => void, onResync: () => void) => {mock.onEvent = onEvent; mock.onResync = onResync; return true;}}));
vi.mock("../../hooks/useOwnerAlerts", () => ({useOwnerAlerts: () => {}}));
vi.mock("../../hooks/useDragDrop", () => ({useDragDrop: (drop: typeof mock.drop) => {mock.drop = drop; return {session: null, startPress: vi.fn(), consumeClick: () => false};}}));
vi.mock("../../hooks/useRunners", async importOriginal => ({...await importOriginal<object>(), useRegisteredRunnersPolling: () => ({snapshot:null,stale:false,error:null,refresh:vi.fn()}), useRunnersPolling: () => ({snapshot: EMPTY_RUNNERS, now})}));
vi.mock("../../hooks/usePRStatus", async importOriginal => ({...await importOriginal<object>(), usePRStatusPolling: () => ({snapshot, now})}));
vi.mock("../toast/ToastContext", () => ({useToast: () => ({error: vi.fn(), success: vi.fn()})}));
vi.mock("./StatsSummary", () => ({StatsSummary: () => null}));
vi.mock("./UsageBars", () => ({UsageBars: () => null}));

it("keeps owner PRs in Your move and sends lane and ordering without a stage override", async () => {
 const host = document.createElement("div"); document.body.append(host);
 const root = createRoot(host);
 const api = {board: vi.fn().mockResolvedValue(baseBoard), moveTicket: mock.move} as unknown as BoardApi;
 await act(async () => root.render(<ApiContext.Provider value={api}><BoardScreen route={{name: "board", ticketId: null, params: new URLSearchParams()}} /></ApiContext.Provider>));
 try {
  expect(host.textContent).toContain("Other work");
  const filter = [...host.querySelectorAll("button")].find(b => b.textContent?.startsWith("Your move"))!;
  expect(filter.textContent).toContain("2");
  await act(async () => filter.click());
  expect(filter.getAttribute("aria-pressed")).toBe("true");
  expect(host.textContent).toContain(prTicket.title);
  expect(host.textContent).toContain("Green change");
  expect(host.textContent).not.toContain("Other work");
  await act(async () => mock.drop!(1, {kind: "list", lane: "in_progress", index: 1}));
  expect(mock.move).toHaveBeenLastCalledWith(1, "in_progress", 2);
  await act(async () => mock.drop!(4, {kind: "pill", lane: "deployed"}));
  expect(mock.move).toHaveBeenLastCalledWith(4, "deployed", null);
  await act(async () => mock.drop!(1, {kind: "list", lane: "deployed", index: 0}));
  expect(mock.move).toHaveBeenLastCalledWith(1, "deployed", null);
 } finally {await act(async () => root.unmount()); host.remove();}
});

it("refreshes deployment and failure from server events and resynchronizes an open sheet", async () => {
 vi.useFakeTimers();
 const host = document.createElement("div"); document.body.append(host); const root = createRoot(host);
 let current = deploymentTicket;
 const api = {
  board: vi.fn(async () => ({lanes, stages: [], repos: [deploymentRepo], tickets: [current]})),
  ticket: vi.fn(async () => current),
  deployTicket: vi.fn(async () => {current = {...current, stage: "deploying", lease: {holder: "release-worker", expires_at: "2099-01-01T00:00:00Z"}}; return current;}),
 };
 const render = async (sheet = false) => act(async () => root.render(
  <ApiContext.Provider value={api as unknown as BoardApi}><CurrentUserContext.Provider value={deploymentUser}>
   <BoardScreen route={sheet ? {name: "ticket", ticketId: 1, params: new URLSearchParams()} : {name: "board", ticketId: null, params: new URLSearchParams()}} />
  </CurrentUserContext.Provider></ApiContext.Provider>
 ));
 const event = {ticket_id: 1} as BoardEvent;
 try {
  await render();
  const action = [...host.querySelectorAll("button")].find(b => b.textContent === "Deploy now")!;
  expect(action).toBeDefined();
  await act(async () => action.click());
  expect(api.deployTicket).toHaveBeenCalledExactlyOnceWith(1);
  // A mutation response alone does not invent a board state; the stream refresh supplies it.
  expect(host.querySelector(".card__activity-label")).toBeNull();
  await act(async () => {mock.onEvent!(event); vi.advanceTimersByTime(300);});
  expect(host.querySelector(".card__activity-label")?.textContent).toBe("Deploying");
  expect(host.querySelector(".card__lease-holder")?.textContent).toBe("release-worker");
  expect([...host.querySelectorAll("button")].some(b => b.textContent === "Deploy now")).toBe(false);
  current = {...current, stage: "done", lease: null, blocked_reason: "Release check failed"};
  await act(async () => {mock.onEvent!(event); vi.advanceTimersByTime(300);});
  expect(host.querySelector(".badge--blocked")?.textContent).toContain("Blocked");
  expect(host.querySelector(".badge--blocked")?.getAttribute("title")).toContain("Release check failed");
  await render(true);
  expect(document.querySelector(".blocked__reason")?.textContent).toBe("Release check failed");
  const reads = api.ticket.mock.calls.length;
  await act(async () => mock.onEvent!({...event, ticket_id: 99}));
  expect(api.ticket).toHaveBeenCalledTimes(reads);
  current = {...current, blocked_reason: ""};
  await act(async () => {mock.onEvent!(event); vi.advanceTimersByTime(300);});
  expect(api.ticket).toHaveBeenCalledTimes(reads + 1);
  expect(document.querySelector(".blocked__reason")).toBeNull();
  expect(document.querySelector(".sheet__header")?.textContent).toContain("Deploy now");
  current = {...current, stage: "deployed", lane: "deployed"};
  const boardReads = api.board.mock.calls.length;
  await act(async () => mock.onResync!());
  expect(api.board).toHaveBeenCalledTimes(boardReads + 1);
  expect(api.ticket).toHaveBeenCalledTimes(reads + 2);
  expect(document.querySelector(".sheet__header")?.textContent).not.toContain("Deploy now");
  expect(document.querySelector(".lane-move__stage")?.textContent).toContain("Deployed");
 } finally {await act(async () => root.unmount()); host.remove(); vi.useRealTimers();}
});
