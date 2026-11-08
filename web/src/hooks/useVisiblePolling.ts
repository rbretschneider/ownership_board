import { useCallback, useEffect, useRef, useState } from "react";

/** Serialize visible-page reads and ignore any response predating a mutation. */
export function useVisiblePolling<T>(load: () => Promise<T>) {
  const [value, setValue] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [receivedAt, setReceivedAt] = useState(0);
  const [now, setNow] = useState(Date.now());
  const generation = useRef(0);
  const request = useRef<{ load: () => Promise<T>; promise: Promise<T> } | null>(null);
  const refreshRef = useRef<() => void>(() => {});
  const refresh = useCallback(() => refreshRef.current(), []);
  const invalidate = useCallback(() => {
    generation.current++;
  }, []);
  useEffect(() => {
    let active = true,
      busy = false,
      again = false;
    const run = async () => {
      if (!active || document.hidden) return;
      if (busy) {
        again = true;
        return;
      }
      busy = true;
      const version = generation.current;
      try {
        // Effect replay may leave a physical request in flight. Reuse it for
        // the same loader, and serialize a changed loader behind it.
        if (request.current && request.current.load !== load) {
          await request.current.promise.catch(() => {});
          again = true;
          return;
        }
        if (!request.current) {
          const promise = Promise.resolve().then(load);
          request.current = { load, promise };
          const clear = () => {
            if (request.current?.promise === promise) request.current = null;
          };
          void promise.then(clear, clear);
        }
        const next = await request.current.promise;
        if (active && version === generation.current) {
          setValue(next);
          setError(null);
          setReceivedAt(Date.now());
        }
      } catch (cause) {
        if (active && version === generation.current) setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        busy = false;
        if (active) {
          setNow(Date.now());
          if (again) {
            again = false;
            void run();
          }
        }
      }
    };
    refreshRef.current = () => {
      void run();
    };
    const visible = () => {
      if (!document.hidden) void run();
    };
    void run();
    const timer = window.setInterval(() => {
      setNow(Date.now());
      if (!busy) void run();
    }, 5000);
    document.addEventListener("visibilitychange", visible);
    return () => {
      active = false;
      generation.current++;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", visible);
      refreshRef.current = () => {};
    };
  }, [load]);
  return { value, error, receivedAt, now, refresh, invalidate, setValue };
}
