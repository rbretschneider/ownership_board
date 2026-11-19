import { useCallback, useEffect, useRef, useState } from "react";
import { useApi } from "../api/ApiContext";
import type { Team } from "../api/types";

/** Build hosts report phases without ticket events, so the view also polls. */
// Short enough that an idle runner's countdown ring restarts soon after its
// 30 s claim lands, rather than sitting at "checking…" for a whole poll.
export const TEAM_POLL_MS = 10_000;

/** Team data for the Team view; refetch keeps what is on screen. */
export interface TeamState {
  team: Team | null;
  error: string | null;
  isLoading: boolean;
  refetch: () => void;
}

/** Loads GET /team and refreshes it on a slow poll while the page is visible. */
export function useTeam(): TeamState {
  const api = useApi();
  const [team, setTeam] = useState<Team | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setLoading] = useState(true);
  const requestSeq = useRef(0);

  const refetch = useCallback(() => {
    const seq = ++requestSeq.current;
    void api.team().then(
      (next) => {
        if (seq !== requestSeq.current) return;
        setTeam(next);
        setError(null);
        setLoading(false);
      },
      (cause: unknown) => {
        if (seq !== requestSeq.current) return;
        setError(cause instanceof Error ? cause.message : String(cause));
        setLoading(false);
      },
    );
  }, [api]);

  useEffect(() => {
    refetch();
    const timer = window.setInterval(() => {
      if (!document.hidden) refetch();
    }, TEAM_POLL_MS);
    return () => window.clearInterval(timer);
  }, [refetch]);

  return { team, error, isLoading, refetch };
}
