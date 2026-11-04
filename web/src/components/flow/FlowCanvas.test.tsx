import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { focusOn, layoutFlow } from "../../lib/flow";
import { liveLikeBoard, NOW } from "../../lib/flowFixture";
import { FlowCanvas } from "./FlowCanvas";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

it("draws lanes, nodes and edges, and dims what a focused milestone does not need", async () => {
  const { tickets, repos } = liveLikeBoard();
  const layout = layoutFlow(tickets, repos, NOW);
  const onSelect = vi.fn();
  const host = document.createElement("div");
  const root = createRoot(host);
  await act(async () => {
    root.render(<FlowCanvas layout={layout} focus={null} showCritical now={NOW} onSelect={onSelect} />);
  });
  expect(host.querySelectorAll(".flow-lane")).toHaveLength(2);
  expect(host.querySelectorAll(".flow-node")).toHaveLength(layout.nodes.size);
  expect(host.querySelectorAll(".flow-edge")).toHaveLength(layout.edges.length);
  expect(host.querySelectorAll(".flow-edge--critical").length).toBeGreaterThan(0);
  const paused = host.querySelector<HTMLButtonElement>('[data-ticket="1"]');
  expect(paused?.className).toContain("flow-node--stuck");
  expect(paused?.textContent).toContain("gates 31");
  expect(paused?.textContent).toContain("↻25");
  paused?.click();
  expect(onSelect).toHaveBeenCalledWith(1);

  await act(async () => {
    root.render(<FlowCanvas layout={layout} focus={focusOn(layout, 51)} showCritical now={NOW} onSelect={onSelect} />);
  });
  expect(host.querySelector('[data-ticket="51"]')?.getAttribute("aria-pressed")).toBe("true");
  expect(host.querySelector('[data-ticket="50"]')?.className).not.toContain("flow-node--dim");
  expect(host.querySelector('[data-ticket="1"]')?.className).toContain("flow-node--dim");
  await act(async () => { root.unmount(); });
});
