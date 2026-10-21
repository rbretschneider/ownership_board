import { useEffect, useRef, useState } from "react";
import type { BoardEvent } from "../api/types";
import { EVENT_STREAM_URL } from "../lib/constants";

const EVENT_NAME = "ticket";
const REFRESH_URL = "/api/v1/auth/refresh";
const RECONNECT_FIRST_DELAY_MS = 2_000;
const RECONNECT_MAX_DELAY_MS = 30_000;

/**
 * Subscribes to the board's server-sent events and reports whether the stream
 * is connected.
 *
 * EventSource retries dropped connections itself, but gives up for good on an
 * HTTP error such as the 401 it gets once the 15-minute access cookie has
 * expired. When that happens this hook renews the session and reopens the
 * stream with backoff, then calls `onResync` because events may have been
 * missed while it was down.
 */
export function useEventStream(onEvent: (event: BoardEvent) => void, onResync: () => void): boolean {
  const handlerRef = useRef(onEvent);
  handlerRef.current = onEvent;
  const resyncRef = useRef(onResync);
  resyncRef.current = onResync;
  const [isConnected, setConnected] = useState(true);

  useEffect(() => {
    let source: EventSource | null = null;
    let retryTimer: number | undefined;
    let delay = RECONNECT_FIRST_DELAY_MS;
    let hasDropped = false;
    let isDisposed = false;

    const onMessage = (message: MessageEvent<string>) => {
      const event = parseEvent(message.data);
      if (event !== null) {
        handlerRef.current(event);
      }
    };

    const scheduleReopen = () => {
      retryTimer = window.setTimeout(() => {
        void renewSession().then(() => {
          if (!isDisposed) {
            open();
          }
        });
      }, delay);
      delay = Math.min(delay * 2, RECONNECT_MAX_DELAY_MS);
    };

    const open = () => {
      source = new EventSource(EVENT_STREAM_URL, { withCredentials: true });
      source.addEventListener(EVENT_NAME, onMessage);
      source.onopen = () => {
        setConnected(true);
        delay = RECONNECT_FIRST_DELAY_MS;
        if (hasDropped) {
          hasDropped = false;
          resyncRef.current();
        }
      };
      source.onerror = () => {
        setConnected(false);
        hasDropped = true;
        if (source !== null && source.readyState === EventSource.CLOSED) {
          source.close();
          scheduleReopen();
        }
      };
    };

    open();
    return () => {
      isDisposed = true;
      window.clearTimeout(retryTimer);
      source?.close();
    };
  }, []);

  return isConnected;
}

/** Renews the access cookie. Failure is fine: the next open simply fails again. */
async function renewSession(): Promise<void> {
  try {
    await fetch(REFRESH_URL, { method: "POST", credentials: "same-origin" });
  } catch {
    // Offline: the reopen will fail and back off further.
  }
}

function parseEvent(data: string): BoardEvent | null {
  try {
    return JSON.parse(data) as BoardEvent;
  } catch {
    // A malformed doorbell is harmless: the next event triggers a refetch anyway.
    return null;
  }
}
