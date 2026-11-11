import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import { ToastProvider } from "../toast/ToastProvider";
import { BuildHostsSection } from "./BuildHostsSection";
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const pending = {
  id: 1,
  name: "Builder",
  os: "linux",
  arch: "amd64",
  status: "pending",
  capabilities: ["gcc"],
  max_parallel: 1,
  enabled: true,
  online: false,
  last_seen_at: null,
  facts: { interactive_session: false },
  agent_version: "1",
};
afterEach(() => {
  vi.useRealTimers();
  document.body.innerHTML = "";
});
async function mount(methods: object) {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  await act(async () =>
    root.render(
      <ApiContext value={methods as BoardApi}>
        <ToastProvider>
          <BuildHostsSection />
        </ToastProvider>
      </ApiContext>,
    ),
  );
  return { host, unmount: () => act(async () => root.unmount()) };
}
async function click(label: string) {
  const button = [...document.querySelectorAll("button")].find((b) => b.textContent === label)!;
  expect(button).toBeTruthy();
  await act(async () => button.click());
}
it("discovers pending hosts by polling and approves an intentionally empty capability list", async () => {
  vi.useFakeTimers();
  const buildHosts = vi.fn().mockResolvedValueOnce([]).mockResolvedValue([pending]);
  const approveBuildHost = vi.fn().mockResolvedValue({ ...pending, status: "approved", capabilities: [] });
  const v = await mount({ buildHosts, approveBuildHost });
  expect(v.host.textContent).not.toContain("Builder");
  expect(v.host.textContent).toContain("No build hosts yet");
  expect(v.host.textContent).not.toContain("Waiting for approval");
  await act(async () => vi.advanceTimersByTime(5000));
  expect(v.host.textContent).toContain("Builder");
  expect(v.host.textContent).toContain("No");
  expect(v.host.textContent).toContain("Unknown");
  await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Remove gcc"]')!.click());
  await click("Approve");
  expect(approveBuildHost).toHaveBeenCalledWith(1, { capabilities: [] });
  await v.unmount();
  const n = buildHosts.mock.calls.length;
  await act(async () => vi.advanceTimersByTime(10000));
  expect(buildHosts).toHaveBeenCalledTimes(n);
});
it("confirms revocation, supports cancellation, and retains revoked records", async () => {
  const revoked = { ...pending, status: "revoked", revoked_at: "2026-01-01T00:00:00Z" };
  const buildHosts = vi.fn().mockResolvedValueOnce([pending]).mockResolvedValue([revoked]);
  const revokeBuildHost = vi.fn().mockResolvedValue(revoked);
  const v = await mount({ buildHosts, revokeBuildHost });
  await click("Reject");
  expect(revokeBuildHost).not.toHaveBeenCalled();
  await click("Cancel");
  expect(revokeBuildHost).not.toHaveBeenCalled();
  await click("Reject");
  await click("Revoke host");
  expect(revokeBuildHost).toHaveBeenCalledWith(1);
  expect(v.host.textContent).toContain("Revoked");
  await v.unmount();
});
it("issues a code once per Add action and discards late responses after closing", async () => {
  let resolve!: (v: unknown) => void;
  const issueBuildHostCode = vi.fn(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  const v = await mount({ buildHosts: vi.fn().mockResolvedValue([]), issueBuildHostCode });
  await click("Add build host");
  expect(issueBuildHostCode).toHaveBeenCalledTimes(1);
  await click("Close");
  await act(async () =>
    resolve({ code: "SECRET", expires_at: "2099-01-01T00:00:00Z", public_base_url: "https://configured.example" }),
  );
  expect(document.body.textContent).not.toContain("SECRET");
  await v.unmount();
});

it("rejects an obsolete poll after approval and preserves pending capability drafts during polling", async () => {
  vi.useFakeTimers();
  let resolvePoll!: (v: unknown) => void;
  const approved = { ...pending, status: "approved", capabilities: [] };
  const buildHosts = vi
    .fn()
    .mockResolvedValueOnce([pending])
    .mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolvePoll = r;
        }),
    )
    .mockResolvedValue([approved]);
  const approveBuildHost = vi.fn().mockResolvedValue(approved);
  const v = await mount({ buildHosts, approveBuildHost });
  await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Remove gcc"]')!.click());
  await act(async () => vi.advanceTimersByTime(5000));
  await click("Approve");
  expect(v.host.textContent).not.toContain("ApproveReject");
  await act(async () => resolvePoll([pending]));
  expect(v.host.textContent).not.toContain("ApproveReject");
  expect(v.host.textContent).not.toContain("Waiting for approval");
  expect([...v.host.querySelectorAll("button")].some((b) => b.textContent === "Edit")).toBe(true);
  await v.unmount();
});
it("keeps failed approval edits for correction and displays the server error", async () => {
  const approveBuildHost = vi.fn().mockRejectedValue(new Error("Host is no longer pending"));
  const v = await mount({ buildHosts: vi.fn().mockResolvedValue([pending]), approveBuildHost });
  await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Remove gcc"]')!.click());
  await click("Approve");
  expect(v.host.querySelector('[role="alert"]')?.textContent).toBe("Host is no longer pending");
  expect(v.host.querySelector('[aria-label="Remove gcc"]')).toBeNull();
  await v.unmount();
});

it("ignores duplicate Add activation and issues a fresh code after reopening", async () => {
  const issueBuildHostCode = vi
    .fn()
    .mockResolvedValue({
      code: "FIRST",
      expires_at: "2099-01-01T00:00:00Z",
      public_base_url: "https://configured.example",
    });
  const v = await mount({ buildHosts: vi.fn().mockResolvedValue([]), issueBuildHostCode });
  const add = [...document.querySelectorAll("button")].find((b) => b.textContent === "Add build host")!;
  await act(async () => {
    add.click();
    add.click();
  });
  expect(issueBuildHostCode).toHaveBeenCalledTimes(1);
  await click("Close");
  issueBuildHostCode.mockResolvedValue({
    code: "SECOND",
    expires_at: "2099-01-01T00:00:00Z",
    public_base_url: "https://configured.example",
  });
  await click("Add build host");
  expect(issueBuildHostCode).toHaveBeenCalledTimes(2);
  expect(document.body.textContent).toContain("SECOND");
  expect(document.body.textContent).not.toContain("FIRST");
  await v.unmount();
});
