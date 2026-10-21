/** The screens reachable through the URL hash. */
export type RouteName = "board" | "ticket" | "settings" | "login";

/** A parsed hash route. */
export interface Route {
  name: RouteName;
  /** Set for the `ticket` route. */
  ticketId: number | null;
  /** Query parameters after `?` in the hash, e.g. `error` or `linked`. */
  params: URLSearchParams;
}

const TICKET_PATTERN = /^\/t\/(\d+)$/;

/** Parses `location.hash` values such as `#/t/42` or `#/login?error=x`. */
export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#/, "");
  const [path = "", query = ""] = raw.split("?", 2);
  const params = new URLSearchParams(query);
  const ticketMatch = TICKET_PATTERN.exec(path);
  if (ticketMatch !== null) {
    return { name: "ticket", ticketId: Number(ticketMatch[1]), params };
  }
  if (path === "/settings") {
    return { name: "settings", ticketId: null, params };
  }
  if (path === "/login") {
    return { name: "login", ticketId: null, params };
  }
  return { name: "board", ticketId: null, params };
}

/** Builds the hash for a ticket. */
export function ticketHash(id: number): string {
  return `#/t/${id}`;
}

/** The hash for the board. */
export const BOARD_HASH = "#/";

/** The hash for settings. */
export const SETTINGS_HASH = "#/settings";

/** Navigates by setting `location.hash`. */
export function navigate(hash: string): void {
  window.location.hash = hash;
}
