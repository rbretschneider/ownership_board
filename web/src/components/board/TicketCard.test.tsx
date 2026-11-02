import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import type { TicketSummary } from "../../api/types";
import { TicketCard } from "./TicketCard";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;
it("turns expired work yellow and hides its model without a board event", async () => {
 vi.useFakeTimers();
 vi.setSystemTime(new Date("2026-11-02T15:00:00Z"));
 const host = document.createElement("div");
 const root = createRoot(host);
 const ticket = {id: 9, repo_id: null, kind: "story", title: "An outcome", stage: "build", lane: "in_progress", is_autonomous: true, needs_owner: false, blocked_reason: "", waiting_on: [], open_questions: 0, pr_url: "", work_model: "gpt-6.1-sol", lease: {holder: "codex_1", expires_at: "2026-11-02T15:00:01Z"}} as unknown as TicketSummary;
 try {
  await act(async () => { root.render(<TicketCard ticket={ticket} stages={[]} repo={undefined} isDragSource={false} onPointerDown={() => {}} shouldIgnoreClick={() => false} />); });
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
