import { createContext, useContext } from "react";
import type { BoardApi } from "./boardApi";

/** Provides the {@link BoardApi} to the component tree. */
export const ApiContext = createContext<BoardApi | null>(null);

/** The injected {@link BoardApi}. Throws outside an `ApiContext` provider. */
export function useApi(): BoardApi {
  const api = useContext(ApiContext);
  if (api === null) {
    throw new Error("useApi must be used inside an ApiContext provider");
  }
  return api;
}
