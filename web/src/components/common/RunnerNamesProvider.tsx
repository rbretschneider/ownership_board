import type { ReactNode } from "react";
import { RunnerNamesContext, useRunnerNamesLoader } from "../../hooks/useRunnerNames";

/** Props for {@link RunnerNamesProvider}. */
export interface RunnerNamesProviderProps {
  /** Load only once someone is signed in. */
  isEnabled: boolean;
  children: ReactNode;
}

/** Loads the runner names once for every page under it. */
export function RunnerNamesProvider({ isEnabled, children }: RunnerNamesProviderProps) {
  const names = useRunnerNamesLoader(isEnabled);
  return <RunnerNamesContext.Provider value={names}>{children}</RunnerNamesContext.Provider>;
}
