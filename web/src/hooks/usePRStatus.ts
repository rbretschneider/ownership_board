import { createContext, useContext, useEffect, useState } from "react";
import { useApi } from "../api/ApiContext";
import type { PRStatusSnapshot } from "../api/types";
import { EMPTY_PR_STATUS } from "../lib/prStatus";

/** The sweep writes every 5 minutes; a 30 s poll shows each run promptly. */
const POLL_MS = 30_000;

/** Polls the board for the merge sweep's view of every open board PR. */
export function usePRStatusPolling(): { snapshot: PRStatusSnapshot; now: number } {
  const api = useApi();
  const [snapshot, setSnapshot] = useState<PRStatusSnapshot>(EMPTY_PR_STATUS);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const value = await api.prs();
        if (active) setSnapshot(value);
      } catch {
        /* Keep the last reading; its age marks it stale. */
      }
      if (active) setNow(Date.now());
    };
    void load();
    const timer = window.setInterval(() => {
      if (!document.hidden) void load();
    }, POLL_MS);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [api]);
  return { snapshot, now };
}

/** Shares one poll between every card. */
export const PRStatusContext = createContext<{ snapshot: PRStatusSnapshot; now: number }>({ snapshot: EMPTY_PR_STATUS, now: 0 });

export function usePRStatus(): { snapshot: PRStatusSnapshot; now: number } {
  return useContext(PRStatusContext);
}
