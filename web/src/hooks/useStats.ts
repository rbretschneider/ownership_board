import { useCallback, useEffect, useRef, useState } from "react";
import { useApi } from "../api/ApiContext";
import type { StatsReport } from "../api/types";

/** A stats report and how loading it went. */
export interface StatsState {
  report: StatsReport | null;
  error: string | null;
  isLoading: boolean;
  /** Refetches without clearing what is on screen. */
  refetch: () => void;
}

/**
 * Loads delivery stats for the last `days` days. With `refreshMs`, refetches
 * on that interval while the tab is visible.
 */
export function useStats(days: number, refreshMs?: number): StatsState {
  const api = useApi();
  const [report, setReport] = useState<StatsReport | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setLoading] = useState(true);
  const requestSeq = useRef(0);

  const load = useCallback(async () => {
    const seq = ++requestSeq.current;
    try {
      const next = await api.stats(days);
      if (seq === requestSeq.current) {
        setReport(next);
        setError(null);
      }
    } catch (cause) {
      if (seq === requestSeq.current) {
        setError(cause instanceof Error ? cause.message : String(cause));
      }
    } finally {
      if (seq === requestSeq.current) {
        setLoading(false);
      }
    }
  }, [api, days]);

  useEffect(() => {
    setLoading(true);
    void load();
  }, [load]);

  useEffect(() => {
    if (refreshMs === undefined) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      if (!document.hidden) {
        void load();
      }
    }, refreshMs);
    return () => window.clearInterval(timer);
  }, [load, refreshMs]);

  const refetch = useCallback(() => void load(), [load]);
  return { report, error, isLoading, refetch };
}
