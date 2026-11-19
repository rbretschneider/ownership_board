import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { Team, User } from "../../api/types";
import { CurrentUserContext } from "../../hooks/useCurrentUser";
import { teamFixture, ticketDetailFixture, TEAM_NOW } from "./teamFixture";
import { TeamPage } from "./TeamPage";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

vi.mock("../../hooks/useEventStream", () => ({ useEventStream: () => true }));

let host: HTMLDivElement;
let root: Root;

const user = (isAdmin: boolean): User => ({ id: 1, name: "Owner", email: "owner@example.test", is_admin: isAdmin, sso_linked: false });

function withJobIds(): Team {
  const team = teamFixture();
  const h = team.build_hosts[0]!;
  h.running = h.running.map((j, i) => ({ ...j, id: String(i + 1).repeat(64), sha: "abcdef0123456789" }));
  h.queued = h.queued.map((j) => ({ ...j, id: "9".repeat(64), sha: "cf04ccdca8b6" }));
  return team;
}

async function mount(isAdmin: boolean) {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(TEAM_NOW);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  const api = {
    team: vi.fn().mockResolvedValue(withJobIds()),
    ticket: vi.fn().mockResolvedValue(ticketDetailFixture()),
    cancelBuildJob: vi.fn().mockResolvedValue(undefined),
  };
  await act(async () => root.render(
    <CurrentUserContext.Provider value={user(isAdmin)}>
      <ApiContext.Provider value={api as unknown as BoardApi}><TeamPage /></ApiContext.Provider>
    </CurrentUserContext.Provider>,
  ));
  return api;
}

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

const cancelButtons = () => Array.from(host.querySelectorAll<HTMLButtonElement>("button.team-cancel"));

it("gives an administrator a Cancel on each running slot and each queued run", async () => {
  await mount(true);
  expect(cancelButtons().map((b) => b.getAttribute("aria-label"))).toEqual([
    "Cancel the run for #12 in slot 1",
    "Cancel the run for #17 in slot 2",
    "Cancel the queued run for #13",
    "Cancel the queued run for #13",
  ]);
  // The waiting ticket's own card under "Waiting for a build slot" offers it too.
  expect(host.querySelector('[aria-label="Waiting for a build slot"] button.team-cancel')?.textContent).toBe("Cancel queued run (cf04ccdc)");
  expect(host.textContent).toContain("#13 waiting 1h 06m");
  expect(host.textContent).toContain("cf04ccdc");
});

it("cancels the queued run after the owner confirms, then refreshes the team", async () => {
  const api = await mount(true);
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  await act(async () => cancelButtons()[2]!.click());
  expect(confirm).toHaveBeenCalledWith(expect.stringContaining("#13 (cf04ccdc)"));
  expect(api.cancelBuildJob).toHaveBeenCalledWith("9".repeat(64));
  expect(api.team).toHaveBeenCalledTimes(2);
});

it("does nothing when the owner declines the confirmation", async () => {
  const api = await mount(true);
  vi.spyOn(window, "confirm").mockReturnValue(false);
  await act(async () => cancelButtons()[0]!.click());
  expect(api.cancelBuildJob).not.toHaveBeenCalled();
});

it("shows no Cancel to someone who is not an administrator", async () => {
  await mount(false);
  expect(cancelButtons()).toHaveLength(0);
  expect(host.textContent).toContain("Queue: #13 waiting 1h 06m");
});
