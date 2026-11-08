import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { BuildHostPairingCode } from "../../api/types";
import { ToastProvider } from "../toast/ToastProvider";
import { BuildHostPairingDialog } from "./BuildHostPairingDialog";
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});
const code = {
  code: "ABC-123",
  expires_at: "2026-01-01T00:00:02Z",
  public_base_url: "https://configured.example/sub/",
};
async function mount(
  issue: Promise<BuildHostPairingCode>,
  issueBuildHostCode = vi.fn().mockResolvedValue({ ...code, code: "NEW" }),
) {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const onClose = vi.fn();
  await act(async () =>
    root.render(
      <ApiContext value={{ issueBuildHostCode } as unknown as BoardApi}>
        <ToastProvider>
          <BuildHostPairingDialog issue={issue} onClose={onClose} />
        </ToastProvider>
      </ApiContext>,
    ),
  );
  return { onClose, issueBuildHostCode, unmount: () => act(async () => root.unmount()) };
}
it("shows configured commands, copies through plain HTTP fallback, expires and replaces explicitly", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-01-01"));
  Object.defineProperty(window, "isSecureContext", { value: false, configurable: true });
  const copy = vi.fn(() => true);
  Object.defineProperty(document, "execCommand", { value: copy, configurable: true });
  const v = await mount(Promise.resolve(code));
  expect(v.issueBuildHostCode).not.toHaveBeenCalled();
  const commands = [...document.querySelectorAll("pre")];
  expect(commands).toHaveLength(3);
  for (const c of commands) {
    expect(c.textContent).toContain("https://configured.example/sub/install/");
    expect(c.textContent).not.toContain("?ABC");
  }
  await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Copy Linux command"]')!.click());
  expect(copy).toHaveBeenCalledWith("copy");
  expect(document.querySelector("textarea")).toBeNull();
  await act(async () => vi.advanceTimersByTime(2000));
  expect(document.body.textContent).toContain("Pairing code expired");
  expect(document.querySelector('[aria-label="Copy Linux command"]')).toBeNull();
  expect(v.issueBuildHostCode).not.toHaveBeenCalled();
  await act(async () =>
    [...document.querySelectorAll("button")].find((b) => b.textContent === "Create replacement code")!.click(),
  );
  expect(v.issueBuildHostCode).toHaveBeenCalledTimes(1);
  expect(document.body.textContent).toContain("NEW");
  await v.unmount();
});
it("explains missing configuration and supports retry after issuance failure", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-01-01"));
  let v = await mount(Promise.resolve({ ...code, public_base_url: "" }));
  expect(document.body.textContent).toContain("PUBLIC_BASE_URL");
  expect(document.querySelectorAll("pre")).toHaveLength(0);
  await v.unmount();
  v = await mount(Promise.reject(new Error("Refused")));
  expect(document.querySelector('[role="alert"]')?.textContent).toContain("Refused");
  expect(document.body.textContent).toContain("Create replacement code");
  await v.unmount();
});
