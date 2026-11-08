import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import type { RepoHealth, StatsRepo } from "../../api/types";
import { HealthTable } from "./HealthTable";

const health: RepoHealth = {
  every: 8, merges_since_check: 8, next_in: 0, last_check_ticket: 42,
  last_checked_at: "2026-03-10T15:00:00Z", open_follow_ups: 3,
};

function repo(id: number, value: RepoHealth | null): StatsRepo {
  return {
    repo_id: id, name: `Repo ${id}`, health: value,
    autonomous: { merged: 0, prs_opened: 0, open: 0 },
    owner: { merged: 0, prs_opened: 0, open: 0 },
  };
}

function render(repos: StatsRepo[]) {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(<HealthTable repos={repos} />);
  return host;
}

it("links the latest review and displays filing time, threshold and follow-ups", () => {
  const host = render([repo(1, health), repo(0, null)]);
  expect(host.querySelector("caption")?.textContent).toBe("Architecture health");
  expect(host.querySelector("a")?.textContent).toBe("#42");
  expect(host.querySelector("a")?.getAttribute("href")).toBe("#/t/42");
  expect(host.querySelector("time")?.getAttribute("datetime")).toBe(health.last_checked_at);
  expect([...host.querySelectorAll("td")].map(c => c.textContent).slice(1)).toEqual(["0", "3"]);
  expect(host.querySelectorAll("tbody tr")).toHaveLength(1);
  expect(host.querySelectorAll('thead th[scope="col"]')).toHaveLength(4);
});

it("preserves history when disabled and renders incomplete history without guessing", () => {
  const host = render([
    repo(1, { ...health, every: 0, next_in: null }),
    repo(2, { ...health, last_check_ticket: null, last_checked_at: null, open_follow_ups: 0 }),
    repo(3, { ...health, last_check_ticket: null }),
    repo(4, { ...health, last_checked_at: null }),
  ]);
  const rows = [...host.querySelectorAll("tbody tr")];
  expect(rows[0]?.textContent).toContain("off");
  expect(rows[0]?.querySelector("a")?.textContent).toBe("#42");
  expect(rows[1]?.textContent).toContain("Never");
  expect(rows[2]?.querySelector("a")).toBeNull();
  expect(rows[2]?.querySelector("time")).not.toBeNull();
  expect(rows[3]?.querySelector("time")).toBeNull();
  expect(rows[3]?.querySelector("a")?.textContent).toBe("#42");
});

it("explains an empty configuration", () => {
  expect(render([]).textContent).toBe("No repositories configured.");
  expect(render([repo(0, null)]).textContent).toBe("No repositories configured.");
});
