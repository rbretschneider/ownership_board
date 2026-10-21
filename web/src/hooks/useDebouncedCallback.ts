import { useCallback, useEffect, useRef } from "react";

/** Returns a stable function that runs `callback` once calls stop for `delayMs`. */
export function useDebouncedCallback(callback: () => void, delayMs: number): () => void {
  const callbackRef = useRef(callback);
  callbackRef.current = callback;
  const timerRef = useRef<number | null>(null);

  useEffect(() => () => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
    }
  }, []);

  return useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
    }
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null;
      callbackRef.current();
    }, delayMs);
  }, [delayMs]);
}
