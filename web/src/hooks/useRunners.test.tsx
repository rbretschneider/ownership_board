import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../api/ApiContext";
import type { BoardApi } from "../api/boardApi";
import { useRegisteredRunnersPolling } from "./useRunners";
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
it("serializes visible polling, retains failures, ages readings and cleans up", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-01-01"));
  let resolve!: (v: unknown) => void;
  const value = { observed_at: "2026-01-01", hosts: [], queued_by_os: { linux: 0, windows: 0, macos: 0 } };
  const registeredRunners = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    )
    .mockRejectedValue(new Error("offline"));
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  function Probe() {
    const reading = useRegisteredRunnersPolling();
    return (
      <div>
        {reading.snapshot ? "retained" : "empty"} {reading.stale ? "stale" : "fresh"} {reading.error}
      </div>
    );
  }
  await act(async () =>
    root.render(
      <ApiContext value={{ registeredRunners } as unknown as BoardApi}>
        <Probe />
      </ApiContext>,
    ),
  );
  expect(registeredRunners).toHaveBeenCalledTimes(1);
  await act(async () => vi.advanceTimersByTime(10000));
  expect(registeredRunners).toHaveBeenCalledTimes(1);
  await act(async () => resolve(value));
  expect(host.textContent).toContain("retained fresh");
  await act(async () => vi.advanceTimersByTime(20000));
  expect(host.textContent).toContain("retained stale");
  const before = registeredRunners.mock.calls.length;
  Object.defineProperty(document, "hidden", { value: true, configurable: true });
  await act(async () => vi.advanceTimersByTime(10000));
  expect(registeredRunners).toHaveBeenCalledTimes(before);
  Object.defineProperty(document, "hidden", { value: false, configurable: true });
  await act(async () => document.dispatchEvent(new Event("visibilitychange")));
  expect(registeredRunners).toHaveBeenCalledTimes(before + 1);
  await act(async () => root.unmount());
  const last = registeredRunners.mock.calls.length;
  await act(async () => vi.advanceTimersByTime(10000));
  expect(registeredRunners).toHaveBeenCalledTimes(last);
  vi.useRealTimers();
  host.remove();
});

it("shares an in-flight read across development effect replay", async () => {
  const { StrictMode } = await import("react");
  const registeredRunners = vi.fn(() => new Promise<never>(() => {}));
  const host = document.createElement("div");
  const root = createRoot(host);
  function Probe() {
    useRegisteredRunnersPolling();
    return null;
  }
  await act(async () =>
    root.render(
      <StrictMode>
        <ApiContext value={{ registeredRunners } as unknown as BoardApi}>
          <Probe />
        </ApiContext>
      </StrictMode>,
    ),
  );
  try {
    expect(registeredRunners).toHaveBeenCalledTimes(1);
  } finally {
    await act(async () => root.unmount());
  }
});
