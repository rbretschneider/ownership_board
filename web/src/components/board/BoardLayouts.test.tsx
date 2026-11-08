import { BoardTestProviders } from "./boardTestProviders";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import type { Stage } from "../../api/types";
import { DesktopBoard } from "./DesktopBoard";
import { MobileBoard } from "./MobileBoard";

import { lanes, prTicket } from "./boardFixture";
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

const stages: Stage[] = [{ id: "pr", lane: "in_progress", label: "PR open", is_owner_review: true }];
const props = { lanes, stages, ticketsByLane: new Map([["in_progress" as const, [prTicket]]]), reposById: new Map(), draggedId: null, target: null, onPointerDown: () => {}, shouldIgnoreClick: () => false };

it("renders four desktop columns and drop lists in supplied API order", () => {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(<BoardTestProviders><DesktopBoard {...props} /></BoardTestProviders>);
  expect([...host.querySelectorAll("h2")].map(h => h.textContent)).toEqual(["Todo", "In progress", "Merged", "Deployed"]);
  expect([...host.querySelectorAll("[data-drop-list]")].map(h => h.getAttribute("data-drop-list"))).toEqual(lanes.map(l => l.id));
  const active = host.querySelector('[data-drop-list="in_progress"]')!;
  expect(active.textContent).toContain(prTicket.title);
  expect(active.querySelector(".stepper")).not.toBeNull();
  expect(host.querySelector("#lane-in_progress")?.closest("section")?.textContent).toContain("PR open 1");
  host.innerHTML = renderToStaticMarkup(<BoardTestProviders><DesktopBoard {...props} lanes={[lanes[3]!, lanes[2]!, lanes[0]!, lanes[1]!]} /></BoardTestProviders>);
  expect([...host.querySelectorAll("h2")].map(h => h.textContent)).toEqual(["Deployed", "Merged", "Todo", "In progress"]);
});

it("renders four mobile switches with counts, owner indication and drop targets", async () => {
  const host = document.createElement("div");
  const root = createRoot(host);
  const select = vi.fn();
  await act(async () => root.render(<BoardTestProviders><MobileBoard {...props} activeLane="in_progress" onSelectLane={select} /></BoardTestProviders>));
  try {
    const pills = [...host.querySelectorAll<HTMLButtonElement>("nav button")];
    expect(pills.map(p => p.querySelector("span:not(.pill__dot)")?.textContent)).toEqual(["Todo", "In progress", "Merged", "Deployed"]);
    expect(pills.map(p => p.getAttribute("aria-label"))).toEqual(["Todo, 0 tickets", "In progress, 1 tickets, 1 need you", "Merged, 0 tickets", "Deployed, 0 tickets"]);
    expect(pills.map(p => p.getAttribute("data-drop-pill"))).toEqual(lanes.map(l => l.id));
    expect(pills.map(p => p.getAttribute("aria-pressed"))).toEqual(["false", "true", "false", "false"]);
    expect(host.querySelector('[data-drop-list="in_progress"]')?.textContent).toContain(prTicket.title);
    await act(async () => pills[3]!.click());
    expect(select).toHaveBeenCalledWith("deployed");
    await act(async () => root.render(<BoardTestProviders><MobileBoard {...props} activeLane="deployed" onSelectLane={select} /></BoardTestProviders>));
    expect(host.querySelector('[data-drop-list="deployed"]')?.textContent).toContain("Deployed work lands here.");
  } finally { await act(async () => root.unmount()); }
});
