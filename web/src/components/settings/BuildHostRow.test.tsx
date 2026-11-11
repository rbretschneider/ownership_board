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
async function pressKey(selector: string, key: string) {
  await act(async () => {
    document.querySelector(selector)!.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true }));
  });
}
it("keeps configuration collapsed until Edit and shows presence, summary and capabilities", async () => {
  const v = await mount({});
  expect(document.querySelector('[type="number"]')).toBeNull();
  expect(v.el.textContent).toContain("Offline · never seen");
  expect(v.el.textContent).toContain("Agent 1");
  expect(v.el.textContent).toContain("2 builds at a time");
  expect(v.el.textContent).toContain("gcc");
  await click("Edit");
  expect(document.querySelector('[type="number"]')).not.toBeNull();
  await click("Cancel");
  expect(document.querySelector('[type="number"]')).toBeNull();
  await v.unmount();
});
it("validates capacity, sends edited fields, keeps failed drafts, and saves enabled with the rest", async () => {
  const editBuildHost = vi
    .fn()
    .mockRejectedValueOnce(new Error("Capacity refused"))
    .mockResolvedValue({ ...host, max_parallel: 3 });
  const v = await mount({ editBuildHost });
  await click("Edit");
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
  expect(editBuildHost).toHaveBeenLastCalledWith(1, { max_parallel: 3 });
  expect(document.querySelector('[type="number"]')).toBeNull();
  editBuildHost.mockResolvedValue({ ...host, enabled: false, max_parallel: 3 });
  await click("Edit");
  await act(async () => document.querySelector<HTMLInputElement>('[role="switch"]')!.click());
  expect(editBuildHost).toHaveBeenCalledTimes(2);
  await click("Save");
  expect(editBuildHost).toHaveBeenLastCalledWith(1, { enabled: false });
  expect(v.el.textContent).toContain("Paused");
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
  await openMenu();
  await click("Show details");
  expect(v.el.textContent).toContain("Events refused");
  expect(v.el.textContent).toContain("Edit");
  await click("Try again");
  expect(v.el.textContent).toContain("Changed gcc: 12 → 13");
  expect(v.el.textContent).toContain("Removed old 1");
  expect(v.el.textContent).toContain("Added cmake 4");
  expect(v.el.textContent).toContain("Host event: future_type");
  await act(async () => vi.advanceTimersByTime(5000));
  expect(buildHostEvents).toHaveBeenCalledTimes(3);
  await openMenu();
  await click("Hide details");
  await act(async () => vi.advanceTimersByTime(5000));
  expect(buildHostEvents).toHaveBeenCalledTimes(3);
  await v.unmount();
});
it("revokes only through the overflow menu", async () => {
  const onRevoke = vi.fn();
  const el = document.createElement("div");
  document.body.append(el);
  const root = createRoot(el);
  await act(async () =>
    root.render(
      <ApiContext value={{} as BoardApi}>
        <BuildHostRow host={host} onMutate={async () => {}} onRevoke={onRevoke} />
      </ApiContext>,
    ),
  );
  expect([...el.querySelectorAll("button")].some((b) => b.textContent?.includes("Revoke"))).toBe(false);
  await openMenu();
  await click("Revoke…");
  expect(onRevoke).toHaveBeenCalledTimes(1);
  await act(async () => root.unmount());
});
it("adds capability chips with Enter, rejects invalid or duplicate tokens, and removes the last with Backspace", async () => {
  const editBuildHost = vi.fn().mockResolvedValue(host);
  const v = await mount({ editBuildHost });
  await click("Edit");
  const field = "input:not([type])";
  await type(field, "Bad token");
  expect(v.el.textContent).toContain("unique lowercase token");
  await pressKey(field, "Enter");
  expect(document.querySelector('[aria-label="Remove Bad token"]')).toBeNull();
  await type(field, "gcc");
  await pressKey(field, "Enter");
  expect(document.querySelectorAll('[aria-label="Remove gcc"]')).toHaveLength(1);
  await type(field, "cmake");
  await pressKey(field, "Enter");
  expect(document.querySelector('[aria-label="Remove cmake"]')).not.toBeNull();
  expect(document.querySelector<HTMLInputElement>(field)!.value).toBe("");
  await pressKey(field, "Backspace");
  expect(document.querySelector('[aria-label="Remove cmake"]')).toBeNull();
  await type(field, "ninja");
  await pressKey(field, "Enter");
  await click("Save");
  expect(editBuildHost).toHaveBeenCalledWith(1, { capabilities: ["gcc", "ninja"] });
  await v.unmount();
});
async function openMenu() {
  await act(async () => document.querySelector<HTMLButtonElement>('[aria-label^="Actions for"]')!.click());
}
