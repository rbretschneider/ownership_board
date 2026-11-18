import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it } from "vitest";
import { EngineSpinner, PixelLoader } from "./EngineSpinner";
import { Spinner } from "./Spinner";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let host: HTMLDivElement;
let root: Root;

beforeEach(() => {
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
});

it("gives every copy its own clip and mask ids, so many spinners can share a page", async () => {
  await act(async () => root.render(<><EngineSpinner size={14} /><EngineSpinner size={14} /></>));
  const ids = [...host.querySelectorAll("[id]")].map((el) => el.id);
  expect(ids.length).toBeGreaterThan(0);
  expect(new Set(ids).size).toBe(ids.length);
  // Every reference points at an id inside its own copy.
  for (const svg of host.querySelectorAll("svg")) {
    const own = new Set([...svg.querySelectorAll("[id]")].map((el) => el.id));
    for (const el of svg.querySelectorAll("[clip-path], [mask]")) {
      const ref = (el.getAttribute("clip-path") ?? el.getAttribute("mask"))!.replace(/^url\(#(.*)\)$/, "$1");
      expect(own.has(ref)).toBe(true);
    }
  }
});

it("is hidden from screen readers unless it carries a label", async () => {
  await act(async () => root.render(<><EngineSpinner /><EngineSpinner label="Building" /></>));
  const [plain, labelled] = [...host.querySelectorAll("svg")];
  expect(plain!.getAttribute("aria-hidden")).toBe("true");
  expect(labelled!.getAttribute("role")).toBe("img");
  expect(labelled!.getAttribute("aria-label")).toBe("Building");
});

it("draws the engine inside the generic spinner and keeps its status label", async () => {
  await act(async () => root.render(<Spinner label="Saving" size={20} />));
  const status = host.querySelector('[role="status"]')!;
  expect(status.textContent).toBe("Saving");
  expect(status.querySelector(".engine-spinner")?.getAttribute("width")).toBe("20");
});

it("keeps the original build matrix available", async () => {
  await act(async () => root.render(<PixelLoader className="extra" />));
  const matrix = host.querySelector(".pixel-loader.extra")!;
  expect(matrix.querySelectorAll("i")).toHaveLength(9);
});
