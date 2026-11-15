import { useEffect, useState } from "react";

/** The current time, refreshed every intervalMs while active; a fixed clock when inactive. */
export function useTick(active: boolean, intervalMs = 30_000): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(timer);
  }, [active, intervalMs]);
  return now;
}
