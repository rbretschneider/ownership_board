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

async function mount(isAdmin: boolean, team: Team = withJobIds()) {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(TEAM_NOW);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  const api = {
    team: vi.fn().mockResolvedValue(team),
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
  ]);
  // #13 shows once: its own card under "Waiting for a build slot", not again in the host's queue line.
  const waiting = host.querySelector('[aria-label="Waiting for a build slot"] .team-queued__item')!;
  expect(waiting.querySelector(".team-slot__head")?.textContent).toContain("Queued");
  expect(waiting.querySelector(".team-slot__head")?.textContent).toContain("cf04ccdc");
  expect(waiting.querySelector(".team-slot__head button.team-cancel")?.textContent).toBe("Cancel");
  expect(host.querySelector('[aria-label="builder-1 queue"]')).toBeNull();
});

it("keeps a queued run in the host's queue when no waiting card shows its ticket", async () => {
  const team = withJobIds();
  team.build_hosts[0]!.queued.push({ id: "8".repeat(64), sha: "0123456789ab", ticket_id: 99, repo: "acme/app", queued_at: team.build_hosts[0]!.queued[0]!.queued_at });
  await mount(true, team);
  const queue = host.querySelector('[aria-label="builder-1 queue"]')!;
  expect(queue.textContent).toContain("#99 waiting");
  expect(queue.textContent).not.toContain("#13");
  expect(queue.querySelector("button.team-cancel")?.getAttribute("aria-label")).toBe("Cancel the queued run for #99");
});

it("cancels the queued run after the owner confirms, then refreshes the team", async () => {
  const api = await mount(true);
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  await act(async () => cancelButtons()[2]!.click());
  expect(confirm).toHaveBeenCalledWith(expect.stringContaining("#13 (cf04ccdc)"));
  expect(api.cancelBuildJob).toHaveBeenCalledWith("9".repeat(64));
  expect(api.team).toHaveBeenCalledTimes(2);
});

it("shows Cancelling… with a spinner while the request runs, then Cancelled and drops the queued run", async () => {
  const api = await mount(true);
  vi.spyOn(window, "confirm").mockReturnValue(true);
  let finish: () => void = () => {};
  api.cancelBuildJob.mockReturnValue(new Promise<void>((resolve) => { finish = resolve; }));
  const waiting = () => host.querySelector('[aria-label="Waiting for a build slot"]')!;
  await act(async () => waiting().querySelector<HTMLButtonElement>("button.team-cancel")!.click());
  const busy = waiting().querySelector<HTMLButtonElement>("button.team-cancel--busy")!;
  expect(busy.disabled).toBe(true);
  expect(busy.textContent).toContain("Cancelling…");
  expect(busy.querySelector("svg, img, span")).not.toBeNull();
  await act(async () => finish());
  expect(waiting().textContent).toContain("Cancelled");
});

it("clears the spinner and says so when the cancel fails", async () => {
  const api = await mount(true);
  vi.spyOn(window, "confirm").mockReturnValue(true);
  const alert = vi.spyOn(window, "alert").mockImplementation(() => {});
  api.cancelBuildJob.mockRejectedValue(new Error("job is already terminal"));
  await act(async () => cancelButtons()[0]!.click());
  expect(alert).toHaveBeenCalledWith("Could not cancel: job is already terminal");
  expect(host.querySelector("button.team-cancel--busy")).toBeNull();
  expect(cancelButtons()[0]!.textContent).toBe("Cancel");
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
  expect(host.querySelector('[aria-label="Waiting for a build slot"] [data-ticket="13"]')).not.toBeNull();
});
