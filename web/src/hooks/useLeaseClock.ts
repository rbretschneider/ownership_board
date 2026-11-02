import { useEffect, useState } from "react";

/** Repaint at lease expiration even when the board receives no new event. */
export function useLeaseClock(expiresAt?: string): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (expiresAt === undefined) return;
    const expires = Date.parse(expiresAt);
    if (!Number.isFinite(expires)) return;
    const timer = window.setTimeout(() => setNow(Date.now()), Math.min(2147483647, Math.max(0, expires - Date.now()) + 10));
    return () => window.clearTimeout(timer);
  }, [expiresAt]);
  return Math.max(now, Date.now());
}
