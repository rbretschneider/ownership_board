import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import { RunnerNamesContext } from "../../hooks/useRunnerNames";
import { runnerDirectory } from "../../lib/runnerNames";
import { ToastProvider } from "../toast/ToastProvider";
import { RunnerNamesSection } from "./RunnerNamesSection";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const directory = runnerDirectory([{ worker: "claude_1", family: "claude", position: 1, display_name: "Michael", assigned_at: null }]);

async function mount(api: Partial<BoardApi>, refresh: () => void) {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  await act(async () => root.render(
    <ApiContext value={api as BoardApi}>
      <ToastProvider>
        <RunnerNamesContext.Provider value={{ directory, refresh }}><RunnerNamesSection /></RunnerNamesContext.Provider>
      </ToastProvider>
    </ApiContext>,
  ));
  return { host, unmount: async () => { await act(async () => root.unmount()); host.remove(); } };
}

async function rename(host: HTMLElement, name: string) {
  const input = host.querySelector<HTMLInputElement>("input")!;
  await act(async () => {
    const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setValue.call(input, name);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => host.querySelector<HTMLButtonElement>('button[type="submit"]')!.click());
}

it("renames a runner and reloads the shared names", async () => {
  const renameRunner = vi.fn().mockResolvedValue({ worker: "claude_1", family: "claude", position: 1, display_name: "Scott", assigned_at: null });
  const refresh = vi.fn();
  const v = await mount({ renameRunner }, refresh);
  try {
    expect(v.host.querySelector("label")?.textContent).toContain("claude_1");
    expect(v.host.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(true);
    await rename(v.host, "  Scott ");
    expect(renameRunner).toHaveBeenCalledWith("claude_1", "Scott");
    expect(refresh).toHaveBeenCalled();
  } finally { await v.unmount(); }
});

it("shows the refusal and keeps the names when a rename fails", async () => {
  const refresh = vi.fn();
  const v = await mount({ renameRunner: vi.fn().mockRejectedValue(new Error("runner claude_2 is already named Dwight")) }, refresh);
  try {
    await rename(v.host, "Dwight");
    expect(document.body.textContent).toContain("Couldn't rename claude_1: runner claude_2 is already named Dwight");
    expect(refresh).not.toHaveBeenCalled();
  } finally { await v.unmount(); }
});
