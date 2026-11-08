import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { useApi } from "../api/ApiContext";
import type { RegisteredRunnersSnapshot, RunnersSnapshot } from "../api/types";
import { EMPTY_RUNNERS } from "../lib/runners";

import { useVisiblePolling } from "./useVisiblePolling";

const POLL_MS = 5_000;

/** Polls the board (never the build host) for the Windows runners snapshot. */
export function useRunnersPolling(): { snapshot: RunnersSnapshot; now: number } {
  const api = useApi();
  const [snapshot, setSnapshot] = useState<RunnersSnapshot>(EMPTY_RUNNERS);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const value = await api.runners();
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

/** Shares one poll between the runners strip and every card. */
export const RunnersContext = createContext<{ snapshot: RunnersSnapshot; now: number }>({ snapshot: EMPTY_RUNNERS, now: 0 });

export function useRunners(): { snapshot: RunnersSnapshot; now: number } {
  return useContext(RunnersContext);
}


export interface RegisteredReading {
 snapshot:RegisteredRunnersSnapshot|null;
 stale:boolean;
 error:string|null;
 refresh:()=>void;
}
export const RegisteredRunnersContext=createContext<RegisteredReading>({snapshot:null,stale:false,error:null,refresh:()=>{}});
export function useRegisteredRunnersPolling():RegisteredReading {
 const api=useApi();const load=useCallback(()=>api.registeredRunners(),[api]);
 const reading=useVisiblePolling(load);
 return {snapshot:reading.value,stale:reading.value!==null&&reading.now-reading.receivedAt>=15000,error:reading.error,refresh:reading.refresh};
}
