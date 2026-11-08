import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { StatsReport } from "../../api/types";
import { parseHash } from "../../lib/hashRoute";
import { StatsSummary } from "../board/StatsSummary";
import { StatsPage } from "./StatsPage";
import { statsReport, statsSegment } from "./statsFixture";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | null = null;

afterEach(async () => {
  await act(async () => root?.unmount());
  root = null;
});

async function render(stats: BoardApi["stats"], node: ReactNode): Promise<HTMLElement> {
  const host = document.createElement("div");
  root = createRoot(host);
  const api = { stats } as unknown as BoardApi;
  await act(async () => root?.render(<ApiContext value={api}>{node}</ApiContext>));
  return host;
}

const report: StatsReport = statsReport(
  statsSegment({ merged_24h: 2, merged: 5, cycle_time: { count: 5, median_hours: 6, p90_hours: 20 }, models: [{ name: "gpt-5", count: 3 }] }),
  statsSegment({ merged_24h: 1, merged: 4, models: [{ name: "claude-opus", count: 1 }] }),
);

it("summarises recent merges on the board, each linking to the stats page", async () => {
  const stats = vi.fn().mockResolvedValue(report);
  const host = await render(stats, <StatsSummary />);

  const links = [...host.querySelectorAll("a")];
  expect(links.map((a) => a.textContent)).toEqual(["3merged · 24h", "9merged · 7d", "6 hcycle · 7d"]);
  expect(links.every((a) => a.getAttribute("href") === "#/stats")).toBe(true);
  expect(links[0]?.title).toContain("Autonomous 2 · yours 1");
  expect(stats).toHaveBeenCalledWith(7);
});

it("offers a way to the stats page when the summary cannot load", async () => {
  const host = await render(vi.fn().mockRejectedValue(new Error("offline")), <StatsSummary />);
  expect(host.querySelector("a")?.textContent).toBe("Stats unavailable");
});

it("compares autonomous tickets with the owner's for the period in the route", async () => {
  const response = statsReport(report.autonomous, report.owner, 90);
  response.repos = [{ repo_id: 1, name: "acme/app", autonomous: { merged: 0, prs_opened: 0, open: 0 }, owner: { merged: 0, prs_opened: 0, open: 0 }, health: { every: 0, merges_since_check: 2, next_in: null, last_check_ticket: 42, last_checked_at: null, open_follow_ups: 1 } }];
  const stats = vi.fn().mockResolvedValue(response);
  const host = await render(stats, <StatsPage route={parseHash("#/stats?days=90")} />);

  expect(stats).toHaveBeenCalledWith(90);
  expect(host.querySelector("h1")?.textContent).toBe("Stats");
  const merged = [...host.querySelectorAll("tr")].find((tr) => tr.querySelector("th")?.textContent === "Merged");
  expect([...(merged?.querySelectorAll("td") ?? [])].map((td) => td.textContent)).toEqual(["5", "4"]);
  expect(host.textContent).toContain("gpt-5");
  expect(host.textContent).toContain("Not recorded yet");
  expect([...host.querySelectorAll("h2")].some(h => h.textContent === "Architecture health")).toBe(true);
  expect(host.querySelector('a[href="#/t/42"]')?.textContent).toBe("#42");
  expect(host.textContent).toContain("off");
  expect(host.querySelector<HTMLInputElement>('input[value="90"]')?.checked).toBe(true);
});

it("shows a retry path when stats fail to load", async () => {
  const stats = vi.fn().mockRejectedValueOnce(new Error("boom")).mockResolvedValue(report);
  const host = await render(stats, <StatsPage route={parseHash("#/stats")} />);

  expect(host.textContent).toContain("Couldn't load stats: boom");
  await act(async () => host.querySelector<HTMLButtonElement>("button")?.click());
  expect(stats).toHaveBeenCalledTimes(2);
  expect(host.textContent).toContain("Delivery, last 7 days");
});
