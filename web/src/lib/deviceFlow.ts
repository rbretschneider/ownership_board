import type { Connection, DeviceFlow, DevicePoll } from "../api/types";

/**
 * The only page the board ever sends a person to for the GitHub device flow.
 * The API also returns a verification URI, but the UI never links to a value
 * that came over the network.
 */
export const GITHUB_DEVICE_URL = "https://github.com/login/device";

/** GitHub's floor for device-flow polling; the UI never polls faster. */
export const MIN_POLL_INTERVAL_MS = 5_000;

const SECOND_MS = 1_000;

/** Where the Connect GitHub dialog is. */
export type DevicePhase =
  | "starting"
  | "waiting"
  | "confirm"
  | "confirming"
  | "syncing"
  | "done"
  | "expired"
  | "denied"
  | "failed";

/** State of the Connect GitHub dialog. */
export interface DeviceState {
  phase: DevicePhase;
  flow: DeviceFlow | null;
  /** Current polling interval; only ever grows. */
  intervalMs: number;
  /** Earliest time (epoch ms) the next poll may be sent. */
  nextPollAt: number;
  login: string;
  replaces: string;
  /** Human-readable problem to show, "" when none. */
  message: string;
  connection: Connection | null;
}

/** Things that happen to the dialog. */
export type DeviceEvent =
  | { type: "restart" }
  | { type: "started"; flow: DeviceFlow; now: number }
  | { type: "startFailed"; message: string }
  | { type: "polled"; poll: DevicePoll; now: number }
  | { type: "pollFailed"; message: string; now: number }
  | { type: "confirming" }
  | { type: "confirmed"; connection: Connection }
  | { type: "confirmFailed"; message: string }
  | { type: "synced"; connection: Connection }
  | { type: "syncTimedOut" };

/** The state before anything has been requested. */
export const initialDeviceState: DeviceState = {
  phase: "starting", flow: null, intervalMs: MIN_POLL_INTERVAL_MS, nextPollAt: 0,
  login: "", replaces: "", message: "", connection: null,
};

/** Pure transition function for the Connect GitHub dialog. */
export function deviceReducer(state: DeviceState, event: DeviceEvent): DeviceState {
  switch (event.type) {
    case "restart":
      return initialDeviceState;
    case "started": {
      const intervalMs = toInterval(event.flow.interval_seconds, MIN_POLL_INTERVAL_MS);
      return { ...initialDeviceState, phase: "waiting", flow: event.flow, intervalMs, nextPollAt: event.now + intervalMs };
    }
    case "startFailed":
      return { ...state, phase: "failed", message: event.message };
    case "polled":
      return applyPoll(state, event.poll, event.now);
    case "pollFailed":
      return { ...state, message: event.message, nextPollAt: event.now + state.intervalMs };
    case "confirming":
      return { ...state, phase: "confirming", message: "" };
    case "confirmed":
      return { ...state, phase: "syncing", connection: event.connection, message: "" };
    case "confirmFailed":
      return { ...state, phase: "confirm", message: event.message };
    case "synced":
      return { ...state, phase: "done", connection: event.connection };
    case "syncTimedOut":
      return { ...state, phase: "done" };
  }
}

/** Milliseconds until the next poll may go out (0 = now). */
export function pollDelay(state: Pick<DeviceState, "nextPollAt">, now: number): number {
  return Math.max(0, state.nextPollAt - now);
}

/** Whether the dialog should be polling GitHub. */
export function isPolling(state: DeviceState): boolean {
  return state.phase === "waiting";
}

/** Formats a remaining duration as m:ss, clamped at zero. */
export function formatCountdown(remainingMs: number): string {
  const totalSeconds = Math.max(0, Math.ceil(remainingMs / SECOND_MS));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

/** Whether a connection's newest sync finished after it was connected. */
export function hasSyncedSince(connection: Connection, connectedAt: string | null): boolean {
  if (connection.last_sync === null) {
    return false;
  }
  if (connectedAt === null) {
    return true;
  }
  return new Date(connection.last_sync.at).getTime() >= new Date(connectedAt).getTime();
}

function applyPoll(state: DeviceState, poll: DevicePoll, now: number): DeviceState {
  const intervalMs = toInterval(poll.interval_seconds, state.intervalMs);
  const next = { ...state, intervalMs, nextPollAt: now + intervalMs, message: "" };
  switch (poll.status) {
    case "pending":
      return next;
    case "confirm":
      return { ...next, phase: "confirm", login: poll.login ?? "", replaces: poll.replaces ?? "" };
    case "expired":
      return { ...next, phase: "expired" };
    case "denied":
      return { ...next, phase: "denied", message: poll.message ?? "" };
  }
}

/** A server interval in ms that never drops below the floor or the current one. */
function toInterval(seconds: number, current: number): number {
  const requested = Number.isFinite(seconds) ? seconds * SECOND_MS : 0;
  return Math.max(requested, current, MIN_POLL_INTERVAL_MS);
}
