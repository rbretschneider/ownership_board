import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import type { Repo, User, TicketSummary } from "../../api/types";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import { CurrentUserContext } from "../../hooks/useCurrentUser";
import { ToastContext } from "../toast/ToastContext";
import { prTicket } from "./boardFixture";
import { TicketCard } from "./TicketCard";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;
it("turns expired work yellow and hides its model without a board event", async () => {
 vi.useFakeTimers();
 vi.setSystemTime(new Date("2026-11-02T15:00:00Z"));
 const host = document.createElement("div");
 const root = createRoot(host);
 const ticket = {id: 9, repo_id: null, kind: "story", title: "An outcome", stage: "build", lane: "in_progress", is_autonomous: true, needs_owner: false, blocked_reason: "", waiting_on: [], open_questions: 0, pr_url: "", work_model: "gpt-6.1-sol", lease: {holder: "codex_1", expires_at: "2026-11-02T15:00:01Z"}} as unknown as TicketSummary;
 try {
  await act(async () => { root.render(<ApiContext.Provider value={{} as BoardApi}><ToastContext.Provider value={{error: vi.fn(), success: vi.fn(), info: vi.fn()}}><TicketCard ticket={ticket} stages={[]} repo={undefined} isDragSource={false} onPointerDown={() => {}} shouldIgnoreClick={() => false} /></ToastContext.Provider></ApiContext.Provider>); });
  expect(host.querySelector(".card--working")).not.toBeNull();
  expect(host.querySelector(".card__model")?.textContent).toContain("gpt-6.1-sol");
  await act(async () => { vi.advanceTimersByTime(1020); });
  expect(host.querySelector(".card--idle")).not.toBeNull();
  expect(host.querySelector(".pixel-loader")).toBeNull();
  expect(host.querySelector(".card__model")).toBeNull();
  expect(host.querySelector(".card__activity")?.textContent).toBe("Queued");
 } finally {
  await act(async () => { root.unmount(); });
  vi.useRealTimers();
 }
});

it("deploys from a sibling control, reports errors, and preserves drag and link interactions", async () => {
 const host = document.createElement("div"); const root = createRoot(host);
 const deployTicket = vi.fn().mockRejectedValueOnce(new Error("Try again")).mockResolvedValue({});
 const error = vi.fn(); const pointer = vi.fn(); const ignore = vi.fn(() => false);
 const ticket = {...prTicket, repo_id: 2, lane: "merged" as const, stage: "done" as const};
 try {
  await act(async () => root.render(<ApiContext.Provider value={{deployTicket} as unknown as BoardApi}><ToastContext.Provider value={{error, success: vi.fn(), info: vi.fn()}}><CurrentUserContext.Provider value={{id: 1} as User}>
   <TicketCard ticket={ticket} stages={[]} repo={{id: 2, deploy_mode: "agent"} as Repo} isDragSource={false} onPointerDown={pointer} shouldIgnoreClick={ignore} />
  </CurrentUserContext.Provider></ToastContext.Provider></ApiContext.Provider>));
  const button = host.querySelector("button")!; const link = host.querySelector("a")!;
  expect(button).not.toBeNull(); expect(button.closest("a")).toBeNull();
  expect(button.parentElement?.parentElement).toBe(link.parentElement);
  expect(link.getAttribute("href")).toBe("#/t/1");
  await act(async () => button.dispatchEvent(new Event("pointerdown", {bubbles: true})));
  expect(pointer).not.toHaveBeenCalled();
  await act(async () => link.dispatchEvent(new Event("pointerdown", {bubbles: true})));
  expect(pointer).toHaveBeenCalledTimes(1);
  expect(link.dispatchEvent(new MouseEvent("click", {bubbles: true, cancelable: true}))).toBe(true);
  expect(link.dispatchEvent(new MouseEvent("auxclick", {bubbles: true, cancelable: true, button: 1}))).toBe(true);
  ignore.mockReturnValue(true);
  expect(link.dispatchEvent(new MouseEvent("click", {bubbles: true, cancelable: true}))).toBe(false);
  await act(async () => button.click());
  expect(deployTicket).not.toHaveBeenCalled();
  ignore.mockReturnValue(false);
  await act(async () => button.click());
  expect(deployTicket).toHaveBeenCalledExactlyOnceWith(1);
  expect(error).toHaveBeenCalledWith("Try again");
  expect(button.disabled).toBe(false);
  await act(async () => button.click());
  expect(deployTicket).toHaveBeenCalledTimes(2);
 } finally {await act(async () => root.unmount());}
});

it("stops deployment animation and hides the holder when the lease expires", async () => {
 vi.useFakeTimers(); vi.setSystemTime(new Date("2026-11-02T15:00:00Z"));
 const host = document.createElement("div"); const root = createRoot(host);
 try {
  await act(async () => root.render(<ApiContext.Provider value={{} as BoardApi}><ToastContext.Provider value={{error: vi.fn(), success: vi.fn(), info: vi.fn()}}>
   <TicketCard ticket={{...prTicket, lane: "merged", stage: "deploying", lease: {holder: "release-worker", expires_at: "2026-11-02T15:00:01Z"}}} stages={[]} repo={undefined} isDragSource={false} onPointerDown={() => {}} shouldIgnoreClick={() => false} />
  </ToastContext.Provider></ApiContext.Provider>));
  expect(host.querySelector(".pixel-loader")).not.toBeNull();
  expect(host.querySelector(".card__lease-holder")?.textContent).toBe("release-worker");
  await act(async () => vi.advanceTimersByTime(1020));
  expect(host.querySelector(".pixel-loader")).toBeNull();
  expect(host.querySelector(".card__lease-holder")).toBeNull();
  expect(host.querySelector(".card__activity-label")?.textContent).toBe("Deploying");
 } finally {await act(async () => root.unmount()); vi.useRealTimers();}
});
