import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { BoardSettings, ManagerStatus } from "../../api/types";
import { ToastProvider } from "../toast/ToastProvider";
import { ManagerSection } from "./ManagerSection";
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

const base = {manager_enabled: false, manager_build_cap: 5, manager_stall_minutes: 120, manager_daily_budget_usd: 10} as BoardSettings;
const status: ManagerStatus = {enabled: false, build_cap: 5, stall_minutes: 120, daily_budget_usd: 10,
  today: {day: "2026-11-15", sessions: 2, cost_usd: 1.5}, remaining_usd: 8.5};

it("shows today's spend and the flagged tickets, and turns the manager on", async () => {
 const editSettings = vi.fn().mockImplementation(async (patch) => ({...base, ...patch}));
 const api = {settings: vi.fn().mockResolvedValue(base), editSettings, managerStatus: vi.fn().mockResolvedValue(status),
  managerSessions: vi.fn().mockResolvedValue({sessions: [{at: "2026-11-15T14:09:16Z", model: "sonnet", cost_usd: 0.25,
   summary: "MANAGER SUMMARY\n- #49 cycling: why: same test"}]}),
  managerFlags: vi.fn().mockResolvedValue({flags: [{ticket_id: 49, repo_id: 31, title: "Parameters", stage: "build",
   kind: "cycling", fingerprint: "cycling:49:x", detail: "the last 2 build-host runs failed the same way"}]})};
 const host = document.createElement("div"); document.body.append(host);
 const root = createRoot(host);
 await act(async () => root.render(<ApiContext value={api as unknown as BoardApi}><ToastProvider><ManagerSection /></ToastProvider></ApiContext>));
 try {
  expect(host.textContent).toContain("Today: $1.50 in 2 session(s); $8.50 left.");
  expect(host.textContent).toContain("#49 Cycling: the last 2 build-host runs failed the same way");
  expect(host.textContent).toContain("sonnet · $0.25");
  expect(host.querySelector("pre")?.textContent).toContain("MANAGER SUMMARY");
  const toggle = host.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
  await act(async () => toggle.click());
  expect(editSettings).toHaveBeenLastCalledWith({manager_enabled: true});
 } finally {await act(async () => root.unmount()); host.remove();}
});
