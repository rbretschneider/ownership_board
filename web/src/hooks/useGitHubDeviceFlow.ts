import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import type { BoardApi } from "../api/boardApi";
import type { Connection } from "../api/types";
import { deviceReducer, hasSyncedSince, initialDeviceState, isPolling, pollDelay, type DeviceEvent, type DeviceState } from "../lib/deviceFlow";

const SYNC_CHECK_MS = 2_000;
const SYNC_WAIT_MS = 60_000;
const COUNTDOWN_TICK_MS = 1_000;

/** What {@link useGitHubDeviceFlow} gives the dialog. */
export interface GitHubDeviceFlow {
  state: DeviceState;
  /** Milliseconds until the code expires. */
  remainingMs: number;
  confirm: () => Promise<void>;
  restart: () => void;
  /** Abandons the flow on the server unless it already finished. */
  cancel: () => void;
}

/** Callbacks for {@link useGitHubDeviceFlow}. */
export interface GitHubDeviceCallbacks {
  /** The connection was stored, and later again once its first sync landed. */
  onConnected: (connection: Connection) => void;
  /** The first sync landed (or failed) after connecting. */
  onFirstSync: (connection: Connection) => void;
}

/**
 * Runs the GitHub device flow: requests a code, polls no faster than the
 * server allows, stops at the confirmation step, and after confirming waits
 * up to a minute for the first repo sync. Everything stops on unmount.
 */
export function useGitHubDeviceFlow(api: BoardApi, callbacks: GitHubDeviceCallbacks): GitHubDeviceFlow {
  const [state, dispatch] = useReducer(deviceReducer, initialDeviceState);
  const stateRef = useRef(state);
  stateRef.current = state;
  const callbacksRef = useRef(callbacks);
  callbacksRef.current = callbacks;
  const isAlive = useIsAlive();

  useStartFlow(api, state.phase === "starting", dispatch);
  usePolling(api, state, dispatch);
  const remainingMs = useCountdown(state);

  const confirm = useCallback(async () => {
    const flow = stateRef.current.flow;
    if (flow === null) {
      return;
    }
    dispatch({ type: "confirming" });
    try {
      const connection = await api.confirmGitHubDevice(flow.flow_id);
      dispatch({ type: "confirmed", connection });
      callbacksRef.current.onConnected(connection);
      const synced = await waitForFirstSync(api, connection, isAlive);
      if (synced === null) {
        dispatch({ type: "syncTimedOut" });
        return;
      }
      dispatch({ type: "synced", connection: synced });
      callbacksRef.current.onConnected(synced);
      callbacksRef.current.onFirstSync(synced);
    } catch (cause) {
      dispatch({ type: "confirmFailed", message: messageOf(cause) });
    }
  }, [api, isAlive]);

  const cancel = useCallback(() => {
    const current = stateRef.current;
    if (current.flow !== null && current.phase !== "syncing" && current.phase !== "done") {
      void api.cancelGitHubDevice(current.flow.flow_id).catch(() => undefined);
    }
  }, [api]);

  const restart = useCallback(() => dispatch({ type: "restart" }), []);
  return { state, remainingMs, confirm, restart, cancel };
}

/** Requests a device code each time the dialog enters the starting phase. */
function useStartFlow(api: BoardApi, isStarting: boolean, dispatch: (event: DeviceEvent) => void): void {
  useEffect(() => {
    if (!isStarting) {
      return;
    }
    let isCurrent = true;
    api.startGitHubDevice()
      .then((flow) => {
        if (isCurrent) {
          dispatch({ type: "started", flow, now: Date.now() });
          return;
        }
        // Superseded (e.g. React's dev double-mount): abandon the extra flow.
        void api.cancelGitHubDevice(flow.flow_id).catch(() => undefined);
      })
      .catch((cause: unknown) => {
        if (isCurrent) {
          dispatch({ type: "startFailed", message: messageOf(cause) });
        }
      });
    return () => {
      isCurrent = false;
    };
  }, [api, isStarting, dispatch]);
}

/** One poll per scheduled slot while waiting; never overlapping, never early. */
function usePolling(api: BoardApi, state: DeviceState, dispatch: (event: DeviceEvent) => void): void {
  const shouldPoll = isPolling(state);
  const flowId = state.flow?.flow_id ?? "";
  const nextPollAt = state.nextPollAt;
  useEffect(() => {
    if (!shouldPoll || flowId === "") {
      return;
    }
    let isCurrent = true;
    const timer = window.setTimeout(() => {
      api.pollGitHubDevice(flowId)
        .then((poll) => {
          if (isCurrent) {
            dispatch({ type: "polled", poll, now: Date.now() });
          }
        })
        .catch((cause: unknown) => {
          if (isCurrent) {
            dispatch({ type: "pollFailed", message: messageOf(cause), now: Date.now() });
          }
        });
    }, pollDelay({ nextPollAt }, Date.now()));
    return () => {
      isCurrent = false;
      window.clearTimeout(timer);
    };
  }, [api, shouldPoll, flowId, nextPollAt, dispatch]);
}

/** Milliseconds until the code expires, ticking once a second while waiting. */
function useCountdown(state: DeviceState): number {
  const expiresAt = state.flow === null ? 0 : new Date(state.flow.expires_at).getTime();
  const [now, setNow] = useState(() => Date.now());
  const isWaiting = state.phase === "waiting";
  useEffect(() => {
    if (!isWaiting) {
      return;
    }
    const timer = window.setInterval(() => setNow(Date.now()), COUNTDOWN_TICK_MS);
    return () => window.clearInterval(timer);
  }, [isWaiting]);
  return Math.max(0, expiresAt - now);
}

/** A stable function reporting whether the component is still mounted. */
function useIsAlive(): () => boolean {
  const aliveRef = useRef(true);
  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);
  return useCallback(() => aliveRef.current, []);
}

/** Checks every few seconds for the first sync after connecting; null on timeout or unmount. */
async function waitForFirstSync(api: BoardApi, connected: Connection, isAlive: () => boolean): Promise<Connection | null> {
  const deadline = Date.now() + SYNC_WAIT_MS;
  while (Date.now() < deadline && isAlive()) {
    await delay(SYNC_CHECK_MS);
    try {
      const current = (await api.connections()).find((c) => c.provider === connected.provider);
      if (current !== undefined && hasSyncedSince(current, connected.connected_at)) {
        return current;
      }
    } catch {
      // A failed check is retried on the next tick; the deadline bounds it.
    }
  }
  return null;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function messageOf(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause);
}
