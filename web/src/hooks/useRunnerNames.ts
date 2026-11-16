import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { useApi } from "../api/ApiContext";
import { EMPTY_RUNNER_DIRECTORY, labelRunner, runnerDirectory, type KnownRunnerName, type RunnerDirectory, type RunnerLabel } from "../lib/runnerNames";

/** Names change only when a new worker starts or the owner renames one. */
export const RUNNER_NAMES_POLL_MS = 60_000;

/** The runner names everyone on the page shares. */
export interface RunnerNamesState {
  directory: RunnerDirectory;
  /** Reloads the names, e.g. after a rename. */
  refresh: () => void;
}

export const RunnerNamesContext = createContext<RunnerNamesState>({ directory: EMPTY_RUNNER_DIRECTORY, refresh: () => {} });

/**
 * Loads GET /runner-names while signed in and refreshes it on a slow poll. A
 * failed read keeps the last names; runners fall back to their raw identity.
 */
export function useRunnerNamesLoader(isEnabled: boolean): RunnerNamesState {
  const api = useApi();
  const [directory, setDirectory] = useState<RunnerDirectory>(EMPTY_RUNNER_DIRECTORY);
  const refresh = useCallback(() => {
    if (!isEnabled) return;
    api.runnerNames().then(
      (body) => setDirectory(runnerDirectory(body.runners)),
      () => { /* Keep the last names; the raw identities still show. */ },
    );
  }, [api, isEnabled]);
  useEffect(() => {
    refresh();
    const timer = window.setInterval(() => {
      if (!document.hidden) refresh();
    }, RUNNER_NAMES_POLL_MS);
    return () => window.clearInterval(timer);
  }, [refresh]);
  return { directory, refresh };
}

/** The shared runner names. */
export function useRunnerNames(): RunnerNamesState {
  return useContext(RunnerNamesContext);
}

/** How the runner with this identity reads; known is a name its payload carries. */
export function useRunnerLabel(identity: string, known?: KnownRunnerName): RunnerLabel {
  return labelRunner(identity, useRunnerNames().directory, known);
}
