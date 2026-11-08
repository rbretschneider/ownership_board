import { createContext, useContext } from "react";
import type { User } from "../api/types";

/** The authenticated human account, absent before login or after session expiry. */
export const CurrentUserContext = createContext<User | null>(null);
export function useCurrentUser(): User | null {
  return useContext(CurrentUserContext);
}
