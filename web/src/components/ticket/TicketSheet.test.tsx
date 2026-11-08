import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { TicketDetail, User } from "../../api/types";
import { CurrentUserContext } from "../../hooks/useCurrentUser";
import { lanes } from "../board/boardFixture";
import { ToastContext } from "../toast/ToastContext";
import { deploymentRepo, deploymentTicket, deploymentUser } from "./deploymentFixture";
import { TicketSheet } from "./TicketSheet";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;
it("applies deployment results, reports failures, and restores deployment after Unblock", async () => {
 const host = document.createElement("div"); document.body.append(host); const root = createRoot(host);
 const queued = {...deploymentTicket, stage: "deploying" as const};
 const api = {ticket: vi.fn().mockResolvedValue(deploymentTicket), deployTicket: vi.fn().mockRejectedValueOnce(new Error("Server unavailable")).mockResolvedValue(queued), unblock: vi.fn().mockResolvedValue(deploymentTicket)};
 const error = vi.fn(); const changed = vi.fn();
 let signal = 0;
 const render = async (user: User | null = deploymentUser) => act(async () => root.render(
  <ApiContext.Provider value={api as unknown as BoardApi}><ToastContext.Provider value={{error, success: vi.fn(), info: vi.fn()}}><CurrentUserContext.Provider value={user}>
   <TicketSheet ticketId={1} lanes={lanes} stages={[]} repos={[deploymentRepo]} tickets={[deploymentTicket]} refreshSignal={signal} onChanged={changed} />
  </CurrentUserContext.Provider></ToastContext.Provider></ApiContext.Provider>
 ));
 const button = (name: string) => [...document.querySelectorAll("button")].find(b => b.textContent?.trim() === name);
 try {
  await render(null); expect(button("Deploy now")).toBeUndefined();
  await render(); expect(button("Deploy now")).toBeDefined();
  await act(async () => button("Deploy now")!.click());
  expect(error).toHaveBeenCalledWith("Server unavailable");
  expect(button("Deploy now")?.disabled).toBe(false);
  await act(async () => button("Deploy now")!.click());
  expect(api.deployTicket).toHaveBeenCalledWith(1);
  expect(changed).toHaveBeenCalledTimes(1);
  expect(button("Deploy now")).toBeUndefined();
  expect(document.querySelector(".lane-move__stage")?.textContent).toContain("Deploying");
  api.ticket.mockResolvedValue({...deploymentTicket, blocked_reason: "Release check failed"});
  signal++; await render();
  expect(document.querySelector(".blocked__reason")?.textContent).toBe("Release check failed");
  expect(button("Deploy now")).toBeUndefined();
  await act(async () => button("Unblock")!.click());
  expect(api.unblock).toHaveBeenCalledWith(1, "");
  expect(button("Deploy now")).toBeDefined();
  const deployed: TicketDetail = {...deploymentTicket, lane: "deployed", stage: "deployed", timeline: [{id: 1, type: "comment", at: "2026-11-01T13:00:00Z", actor: {kind: "agent", name: "release-worker"}, body: "Release completed successfully.", data: {}}]};
  api.ticket.mockResolvedValue(deployed); signal++; await render();
  expect(document.querySelector(".timeline")?.textContent).toContain("Release completed successfully.");
  expect(button("Deploy now")).toBeUndefined();
 } finally {await act(async () => root.unmount()); host.remove();}
});
