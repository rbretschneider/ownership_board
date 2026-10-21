import { useCallback, useEffect, useRef, useState } from "react";
import { useApi } from "../api/ApiContext";
import { ApiError } from "../api/ApiError";
import type { TicketDetail } from "../api/types";

const HTTP_NOT_FOUND = 404;

/** One ticket's detail plus a way to replace it after a mutation. */
export interface TicketState {
  ticket: TicketDetail | null;
  error: string | null;
  isMissing: boolean;
  isLoading: boolean;
  reload: () => void;
  /** Replaces the ticket with a server response from a mutation. */
  accept: (ticket: TicketDetail) => void;
}

/** Loads a ticket by id. */
export function useTicket(id: number): TicketState {
  const api = useApi();
  const [ticket, setTicket] = useState<TicketDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isMissing, setMissing] = useState(false);
  const [isLoading, setLoading] = useState(true);
  const seq = useRef(0);

  const load = useCallback(async () => {
    const mine = ++seq.current;
    try {
      const next = await api.ticket(id);
      if (mine === seq.current) {
        setTicket(next);
        setError(null);
        setMissing(false);
      }
    } catch (cause) {
      if (mine === seq.current) {
        setMissing(cause instanceof ApiError && cause.status === HTTP_NOT_FOUND);
        setError(cause instanceof Error ? cause.message : String(cause));
      }
    } finally {
      if (mine === seq.current) {
        setLoading(false);
      }
    }
  }, [api, id]);

  useEffect(() => {
    setTicket(null);
    setLoading(true);
    void load();
  }, [load]);

  const accept = useCallback((next: TicketDetail) => {
    seq.current++;
    setTicket(next);
    setError(null);
  }, []);

  const reload = useCallback(() => {
    void load();
  }, [load]);

  return { ticket, error, isMissing, isLoading, reload, accept };
}
