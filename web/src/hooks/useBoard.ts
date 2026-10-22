import { useCallback, useEffect, useRef, useState } from "react";
import { useApi } from "../api/ApiContext";
import type { RepoFilter } from "../api/boardApi";
import type { Board, LaneId } from "../api/types";
import { encodeRepoFilter } from "../lib/repos";
import { applyMove } from "../lib/position";

/** Board data plus the operations the board screen needs. */
export interface BoardState {
  board: Board | null;
  error: string | null;
  isLoading: boolean;
  /** Refetches without clearing what is on screen. */
  refetch: () => void;
  /**
   * Moves a card optimistically and persists it. Resolves to an error message
   * when the server rejected the move (the board is reverted), else null.
   */
  move: (ticketId: number, lane: LaneId, afterId: number | null) => Promise<string | null>;
}

/** Loads the board for a repository filter. */
export function useBoard(filter: RepoFilter): BoardState {
  const filterKey = encodeRepoFilter(filter);
  const api = useApi();
  const [board, setBoard] = useState<Board | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setLoading] = useState(true);
  const requestSeq = useRef(0);

  const load = useCallback(async () => {
    const seq = ++requestSeq.current;
    try {
      const next = await api.board(filter);
      if (seq === requestSeq.current) {
        setBoard(next);
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
    // Keyed on filterKey (the filter by value) so a new-but-equal object does not refetch.
  }, [api, filterKey]);

  useEffect(() => {
    setLoading(true);
    void load();
  }, [load]);

  const move = useCallback(async (ticketId: number, lane: LaneId, afterId: number | null) => {
    const snapshot = board;
    if (snapshot === null) {
      return null;
    }
    // Bump the sequence so a refetch already in flight cannot overwrite the optimistic state.
    requestSeq.current++;
    // Only the position is optimistic: the server picks the stage, and the refetch below shows it.
    const moved = applyMove(snapshot.tickets, ticketId, lane, afterId);
    setBoard({ ...snapshot, tickets: moved });
    try {
      await api.moveTicket(ticketId, lane, afterId);
      void load();
      return null;
    } catch (cause) {
      setBoard(snapshot);
      return cause instanceof Error ? cause.message : String(cause);
    }
  }, [api, board, load]);

  const refetch = useCallback(() => {
    void load();
  }, [load]);

  return { board, error, isLoading, refetch, move };
}
