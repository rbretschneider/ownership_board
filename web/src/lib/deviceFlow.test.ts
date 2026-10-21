import { describe, expect, it } from "vitest";
import type { Connection, DeviceFlow, DevicePoll } from "../api/types";
import {
  deviceReducer, formatCountdown, GITHUB_DEVICE_URL, hasSyncedSince, initialDeviceState,
  isPolling, MIN_POLL_INTERVAL_MS, pollDelay, type DeviceState,
} from "./deviceFlow";

const T0 = 1_000_000;
const flow: DeviceFlow = {
  flow_id: "f1", user_code: "ABCD-1234", verification_uri: "https://github.com/login/device",
  expires_at: new Date(T0 + 900_000).toISOString(), interval_seconds: 5,
};

function started(): DeviceState {
  return deviceReducer(initialDeviceState, { type: "started", flow, now: T0 });
}

function poll(state: DeviceState, result: DevicePoll, now: number): DeviceState {
  return deviceReducer(state, { type: "polled", poll: result, now });
}

function connection(overrides: Partial<Connection> = {}): Connection {
  return {
    provider: "github", label: "GitHub", is_available: true, is_connected: true, status: "connected",
    managed_by: "settings", account: "octo", base_url: "", email: "", connected_at: "2026-01-01T10:00:00Z",
    repo_count: 3, last_sync: null, ...overrides,
  };
}

describe("device flow state machine", () => {
  it("starts waiting and schedules the first poll one interval out", () => {
    const state = started();
    expect(state.phase).toBe("waiting");
    expect(isPolling(state)).toBe(true);
    expect(pollDelay(state, T0)).toBe(5_000);
  });

  it("keeps waiting on pending and schedules from the response time", () => {
    const state = poll(started(), { status: "pending", interval_seconds: 5 }, T0 + 5_000);
    expect(state.phase).toBe("waiting");
    expect(pollDelay(state, T0 + 5_000)).toBe(5_000);
  });

  it("slows down when the server raises the interval", () => {
    const state = poll(started(), { status: "pending", interval_seconds: 10 }, T0 + 5_000);
    expect(state.intervalMs).toBe(10_000);
    expect(pollDelay(state, T0 + 6_000)).toBe(9_000);
  });

  it("never polls faster than the current interval or GitHub's floor", () => {
    const slowed = poll(started(), { status: "pending", interval_seconds: 15 }, T0);
    const after = poll(slowed, { status: "pending", interval_seconds: 1 }, T0 + 15_000);
    expect(after.intervalMs).toBe(15_000);
    const tiny = deviceReducer(initialDeviceState, { type: "started", flow: { ...flow, interval_seconds: 0 }, now: T0 });
    expect(tiny.intervalMs).toBe(MIN_POLL_INTERVAL_MS);
  });

  it("stops polling and asks for confirmation with the login", () => {
    const state = poll(started(), { status: "confirm", interval_seconds: 5, login: "octo", replaces: "old" }, T0 + 5_000);
    expect(state.phase).toBe("confirm");
    expect(isPolling(state)).toBe(false);
    expect(state.login).toBe("octo");
    expect(state.replaces).toBe("old");
  });

  it("reports expiry and denial", () => {
    expect(poll(started(), { status: "expired", interval_seconds: 5 }, T0).phase).toBe("expired");
    const denied = poll(started(), { status: "denied", interval_seconds: 5, message: "not allowed" }, T0);
    expect(denied.phase).toBe("denied");
    expect(denied.message).toBe("not allowed");
  });

  it("keeps polling after a network failure but backs off a full interval", () => {
    const state = deviceReducer(started(), { type: "pollFailed", message: "offline", now: T0 + 5_000 });
    expect(state.phase).toBe("waiting");
    expect(state.message).toBe("offline");
    expect(pollDelay(state, T0 + 5_000)).toBe(5_000);
  });

  it("returns to the confirm step when confirming fails", () => {
    const confirm = poll(started(), { status: "confirm", interval_seconds: 5, login: "octo" }, T0);
    const failed = deviceReducer(deviceReducer(confirm, { type: "confirming" }), { type: "confirmFailed", message: "boom" });
    expect(failed.phase).toBe("confirm");
    expect(failed.message).toBe("boom");
  });

  it("moves through syncing to done", () => {
    const syncing = deviceReducer(started(), { type: "confirmed", connection: connection() });
    expect(syncing.phase).toBe("syncing");
    expect(deviceReducer(syncing, { type: "synced", connection: connection({ repo_count: 9 }) }).connection?.repo_count).toBe(9);
  });
});

describe("device flow helpers", () => {
  it("links only to GitHub's device page", () => {
    expect(GITHUB_DEVICE_URL).toBe("https://github.com/login/device");
  });

  it("formats the countdown as m:ss", () => {
    expect(formatCountdown(899_100)).toBe("15:00");
    expect(formatCountdown(65_000)).toBe("1:05");
    expect(formatCountdown(-10)).toBe("0:00");
  });

  it("detects a sync that finished after connecting", () => {
    const connectedAt = "2026-01-01T10:00:00Z";
    expect(hasSyncedSince(connection({ last_sync: null }), connectedAt)).toBe(false);
    expect(hasSyncedSince(connection({ last_sync: { at: "2026-01-01T09:00:00Z", error: "" } }), connectedAt)).toBe(false);
    expect(hasSyncedSince(connection({ last_sync: { at: "2026-01-01T10:00:05Z", error: "" } }), connectedAt)).toBe(true);
  });
});
