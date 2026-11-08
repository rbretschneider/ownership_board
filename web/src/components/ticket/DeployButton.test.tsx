import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import type { Repo, User } from "../../api/types";
import { CurrentUserContext } from "../../hooks/useCurrentUser";
import { prTicket } from "../board/boardFixture";
import { DeployButton } from "./DeployButton";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;
it("hides ineligible actions and prevents duplicate requests until retry is possible", async () => {
  const host = document.createElement("div");
  const root = createRoot(host);
  const ticket = {...prTicket, repo_id: 2, lane: "merged" as const, stage: "done" as const};
  const repo = {id: 2, deploy_mode: "agent"} as Repo;
  const user = {id: 1, is_admin: false} as User;
  let resolve!: (ok: boolean) => void;
  const deploy = vi.fn(() => new Promise<boolean>(r => {resolve = r;}));
  const render = async (viewer: User | null, stage = ticket.stage) => act(async () => root.render(
    <CurrentUserContext.Provider value={viewer}><DeployButton ticket={{...ticket, stage}} repo={repo} onDeploy={deploy} /></CurrentUserContext.Provider>
  ));
  try {
    await render(null);
    expect(host.querySelector("button")).toBeNull();
    await render(user);
    const button = host.querySelector("button")!;
    expect(button.textContent).toContain("Deploy now");
    await act(async () => {button.click(); button.click();});
    expect(deploy).toHaveBeenCalledTimes(1);
    expect(button.disabled).toBe(true);
    await act(async () => resolve(false));
    expect(button.disabled).toBe(false);
    await act(async () => button.click());
    expect(deploy).toHaveBeenCalledTimes(2);
    await act(async () => resolve(true));
    await act(async () => root.render(<CurrentUserContext.Provider value={user}><DeployButton ticket={{...ticket, stage: "deploying"}} repo={repo} onDeploy={deploy} /></CurrentUserContext.Provider>));
    expect(host.querySelector("button")).toBeNull();
  } finally {await act(async () => root.unmount());}
});
