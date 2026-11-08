import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { BuildHost } from "../../api/types";
import { BuildHostRow } from "./BuildHostRow";
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const host: BuildHost = {
  id: 1,
  name: "Builder",
  os: "linux",
  arch: "amd64",
  status: "approved",
  capabilities: ["gcc"],
  max_parallel: 2,
  enabled: true,
  online: false,
  last_seen_at: null,
  agent_version: "1",
  facts: {},
};
afterEach(() => {
  vi.useRealTimers();
  document.body.innerHTML = "";
});
async function mount(methods: object) {
  const el = document.createElement("div");
  document.body.append(el);
  const root = createRoot(el);
  let current = host;
  const render = () =>
    root.render(
      <ApiContext value={methods as BoardApi}>
        <BuildHostRow
          host={current}
          onMutate={async (_, op) => {
            current = await op();
            render();
          }}
          onRevoke={() => {}}
        />
      </ApiContext>,
    );
  await act(async () => render());
  return {
    el,
    rerender: async (h: BuildHost) => {
      current = h;
      await act(async () => render());
    },
    unmount: () => act(async () => root.unmount()),
  };
}
async function click(text: string) {
  await act(async () => [...document.querySelectorAll("button")].find((b) => b.textContent === text)!.click());
}
async function type(selector: string, value: string) {
  const el = document.querySelector<HTMLInputElement>(selector)!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
it("validates capacity, sends edited fields, keeps failed drafts, and persists enabled independently", async () => {
  const editBuildHost = vi
    .fn()
    .mockRejectedValueOnce(new Error("Capacity refused"))
    .mockResolvedValue({ ...host, max_parallel: 3 });
  const v = await mount({ editBuildHost });
  await type('[type="number"]', "0");
  expect(document.querySelector('[role="alert"]')?.textContent).toContain("1 through 8");
  await click("Save");
  expect(editBuildHost).not.toHaveBeenCalled();
  await type('[type="number"]', "3");
  await click("Save");
  expect(editBuildHost).toHaveBeenCalledWith(1, { max_parallel: 3 });
  expect(document.querySelector('[role="alert"]')?.textContent).toBe("Capacity refused");
  expect(document.querySelector<HTMLInputElement>('[type="number"]')!.value).toBe("3");
  await v.rerender({ ...host, max_parallel: 4, capabilities: ["clang"] });
  expect(document.querySelector<HTMLInputElement>('[type="number"]')!.value).toBe("3");
  expect(v.el.textContent).toContain("clang");
  await click("Save");
  editBuildHost.mockResolvedValue({ ...host, enabled: false, max_parallel: 3 });
  await act(async () => document.querySelector<HTMLInputElement>('[role="switch"]')!.click());
  expect(editBuildHost).toHaveBeenLastCalledWith(1, { enabled: false });
  expect(document.querySelector<HTMLInputElement>('[role="switch"]')!.checked).toBe(false);
  await v.unmount();
});
it("shows bounded recent toolchain diffs, unknown events, errors and retry without losing controls", async () => {
  vi.useFakeTimers();
  const buildHostEvents = vi
    .fn()
    .mockRejectedValueOnce(new Error("Events refused"))
    .mockResolvedValue([
      {
        id: 1,
        event_type: "toolchains_changed",
        created_at: "2026-01-01",
        data: { before: { gcc: "12", old: "1" }, after: { gcc: "13", cmake: "4" } },
      },
      { id: 2, event_type: "future_type", created_at: "2026-01-01", data: {} },
    ]);
  const v = await mount({ buildHostEvents });
  expect(buildHostEvents).not.toHaveBeenCalled();
  await click("Show details");
  expect(v.el.textContent).toContain("Events refused");
  expect(v.el.textContent).toContain("Revoke");
  await click("Try again");
  expect(v.el.textContent).toContain("Changed gcc: 12 → 13");
  expect(v.el.textContent).toContain("Removed old 1");
  expect(v.el.textContent).toContain("Added cmake 4");
  expect(v.el.textContent).toContain("Host event: future_type");
  await act(async () => vi.advanceTimersByTime(5000));
  expect(buildHostEvents).toHaveBeenCalledTimes(3);
  await click("Hide details");
  await act(async () => vi.advanceTimersByTime(5000));
  expect(buildHostEvents).toHaveBeenCalledTimes(3);
  await v.unmount();
});
it("rejects invalid or duplicate capability tokens and bounds the number of chips", async () => {
  const v = await mount({});
  await type("input:not([type])", "Bad token");
  expect(v.el.textContent).toContain("unique lowercase token");
  await click("Add capability");
  expect(document.querySelector('[aria-label="Remove Bad token"]')).toBeNull();
  await v.unmount();
});
