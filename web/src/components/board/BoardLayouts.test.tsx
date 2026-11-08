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

it("renders three desktop columns and drop lists in supplied API order", () => {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(<DesktopBoard {...props} />);
  expect([...host.querySelectorAll("h2")].map(h => h.textContent)).toEqual(["Todo", "In progress", "Merged"]);
  expect([...host.querySelectorAll("[data-drop-list]")].map(h => h.getAttribute("data-drop-list"))).toEqual(lanes.map(l => l.id));
  const active = host.querySelector('[data-drop-list="in_progress"]')!;
  expect(active.textContent).toContain(prTicket.title);
  expect(active.querySelector(".stepper")).not.toBeNull();
  expect(host.querySelector("#lane-in_progress")?.closest("section")?.textContent).toContain("PR open 1");
  host.innerHTML = renderToStaticMarkup(<DesktopBoard {...props} lanes={[lanes[2]!, lanes[0]!, lanes[1]!]} />);
  expect([...host.querySelectorAll("h2")].map(h => h.textContent)).toEqual(["Merged", "Todo", "In progress"]);
});

it("renders three mobile switches with counts, owner indication and drop targets", async () => {
  const host = document.createElement("div");
  const root = createRoot(host);
  const select = vi.fn();
  await act(async () => root.render(<MobileBoard {...props} activeLane="in_progress" onSelectLane={select} />));
  try {
    const pills = [...host.querySelectorAll<HTMLButtonElement>("nav button")];
    expect(pills.map(p => p.querySelector("span:not(.pill__dot)")?.textContent)).toEqual(["Todo", "In progress", "Merged"]);
    expect(pills.map(p => p.getAttribute("aria-label"))).toEqual(["Todo, 0 tickets", "In progress, 1 tickets, 1 need you", "Merged, 0 tickets"]);
    expect(pills.map(p => p.getAttribute("data-drop-pill"))).toEqual(lanes.map(l => l.id));
    expect(pills.map(p => p.getAttribute("aria-pressed"))).toEqual(["false", "true", "false"]);
    expect(host.querySelector('[data-drop-list="in_progress"]')?.textContent).toContain(prTicket.title);
    await act(async () => pills[2]!.click());
    expect(select).toHaveBeenCalledWith("merged");
  } finally { await act(async () => root.unmount()); }
});
