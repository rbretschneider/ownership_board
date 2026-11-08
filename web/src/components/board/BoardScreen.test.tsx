import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import type { PRStatusSnapshot } from "../../api/types";
import type { DropTarget } from "../../lib/dropTargets";
import { EMPTY_RUNNERS } from "../../lib/runners";
import { lanes, prTicket } from "./boardFixture";
import { BoardScreen } from "./BoardScreen";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

const mock = vi.hoisted(() => ({ move: vi.fn().mockResolvedValue(null), drop: null as null | ((id: number, target: DropTarget) => void) }));
const now = Date.parse("2026-11-08T12:00:00Z");
const snapshot: PRStatusSnapshot = {observed_at: new Date(now).toISOString(), prs: [{
 ticket_id: 2, url: "https://github.com/acme/app/pull/2", verdict: "green_owner", reason: "", may_merge: false, acts: false,
 gl_waived: false, draft: false, mergeable: "MERGEABLE", review: "", head: "abc123", unresolved: false, manual_check: false, checks: [],
}]};
vi.mock("../../hooks/useBoard", () => ({useBoard: () => ({board: {lanes, stages: [], repos: [], tickets: [prTicket, {...prTicket, id: 2, title: "Green change", needs_owner: false, pr_url: snapshot.prs[0]!.url}, {...prTicket, id: 3, title: "Other work", needs_owner: false, stage: "build"}, {...prTicket, id: 4, lane: "todo", stage: "ready", needs_owner: false}]}, isLoading: false, error: null, refetch: vi.fn(), move: mock.move})}));
vi.mock("../../hooks/useMediaQuery", () => ({useMediaQuery: () => true}));
vi.mock("../../hooks/useEventStream", () => ({useEventStream: () => true}));
vi.mock("../../hooks/useOwnerAlerts", () => ({useOwnerAlerts: () => {}}));
vi.mock("../../hooks/useDragDrop", () => ({useDragDrop: (drop: typeof mock.drop) => {mock.drop = drop; return {session: null, startPress: vi.fn(), consumeClick: () => false};}}));
vi.mock("../../hooks/useRunners", async importOriginal => ({...await importOriginal<object>(), useRunnersPolling: () => ({snapshot: EMPTY_RUNNERS, now})}));
vi.mock("../../hooks/usePRStatus", async importOriginal => ({...await importOriginal<object>(), usePRStatusPolling: () => ({snapshot, now})}));
vi.mock("../toast/ToastContext", () => ({useToast: () => ({error: vi.fn(), success: vi.fn()})}));
vi.mock("./StatsSummary", () => ({StatsSummary: () => null}));
vi.mock("./UsageBars", () => ({UsageBars: () => null}));

it("keeps owner PRs in Your move and sends lane and ordering without a stage override", async () => {
 const host = document.createElement("div"); document.body.append(host);
 const root = createRoot(host);
 await act(async () => root.render(<BoardScreen route={{name: "board", ticketId: null, params: new URLSearchParams()}} />));
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
  await act(async () => mock.drop!(4, {kind: "pill", lane: "in_progress"}));
  expect(mock.move).toHaveBeenLastCalledWith(4, "in_progress", null);
 } finally {await act(async () => root.unmount()); host.remove();}
});
