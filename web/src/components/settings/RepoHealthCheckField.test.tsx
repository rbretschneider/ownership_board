import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { Repo } from "../../api/types";
import { ToastProvider } from "../toast/ToastProvider";
import { RepoHealthCheckField } from "./RepoHealthCheckField";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const repo = { id: 3, full_name: "acme/api", health_check_every: 8 } as Repo;

async function mount(editRepo: ReturnType<typeof vi.fn>) {
  const onUpdated = vi.fn();
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  await act(async () =>
    root.render(
      <ApiContext value={{ editRepo } as unknown as BoardApi}>
        <ToastProvider><RepoHealthCheckField repo={repo} onUpdated={onUpdated} /></ToastProvider>
      </ApiContext>,
    ),
  );
  const input = host.querySelector<HTMLInputElement>("input")!;
  const type = async (value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
  };
  const blur = () => act(async () => { input.dispatchEvent(new FocusEvent("focusout", { bubbles: true })); });
  return { host, input, type, blur, onUpdated, unmount: () => act(async () => root.unmount()) };
}

it("shows the label and the saved value", async () => {
  const view = await mount(vi.fn());
  expect(view.host.textContent).toContain("Architecture check every N merges (0 = off)");
  expect(view.input.value).toBe("8");
  expect(view.input.labels?.[0]?.textContent).toContain("Architecture check");
  await view.unmount();
});

it("saves once on blur, not on each keystroke", async () => {
  const editRepo = vi.fn().mockResolvedValue({ ...repo, health_check_every: 12 });
  const view = await mount(editRepo);
  await view.type("1");
  await view.type("12");
  expect(editRepo).not.toHaveBeenCalled();
  await view.blur();
  expect(editRepo).toHaveBeenCalledTimes(1);
  expect(editRepo).toHaveBeenCalledWith(3, { health_check_every: 12 });
  expect(view.onUpdated).toHaveBeenCalledWith({ ...repo, health_check_every: 12 });
  await view.unmount();
});

it("saves on Enter", async () => {
  const editRepo = vi.fn().mockResolvedValue({ ...repo, health_check_every: 0 });
  const view = await mount(editRepo);
  await view.type("0");
  await act(async () => { view.input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })); });
  expect(editRepo).toHaveBeenCalledWith(3, { health_check_every: 0 });
  await view.unmount();
});

it("does not send an unchanged value", async () => {
  const editRepo = vi.fn();
  const view = await mount(editRepo);
  await view.type("8");
  await view.blur();
  expect(editRepo).not.toHaveBeenCalled();
  await view.unmount();
});

it("shows an inline error for an out-of-range value without calling the API", async () => {
  const editRepo = vi.fn();
  const view = await mount(editRepo);
  await view.type("51");
  await view.blur();
  const alert = view.host.querySelector('[role="alert"]')!;
  expect(alert.textContent).toContain("0 to 50");
  expect(view.input.getAttribute("aria-describedby")).toBe(alert.id);
  expect(editRepo).not.toHaveBeenCalled();
  await view.unmount();
});

it("shows the server message and reverts when the server refuses", async () => {
  const editRepo = vi.fn().mockRejectedValue(new Error("only a person can edit repos"));
  const view = await mount(editRepo);
  await view.type("20");
  await view.blur();
  expect(view.host.querySelector('[role="alert"]')!.textContent).toContain("only a person can edit repos");
  expect(view.input.value).toBe("8");
  await view.unmount();
});
