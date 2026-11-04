/** The screens reachable through the URL hash. */
export type RouteName = "board" | "ticket" | "settings" | "stats" | "login";

/** A parsed hash route. */
export interface Route {
  name: RouteName;
  /** Set for the `ticket` route. */
  ticketId: number | null;
  /** Query parameters after `?` in the hash, e.g. `error` or `linked`. */
  params: URLSearchParams;
}

const TICKET_PATTERN = /^\/t\/(\d+)$/;

/** Plain paths that name a screen on their own. */
const PAGE_PATHS: Partial<Record<string, RouteName>> = {
  "/settings": "settings",
  "/stats": "stats",
  "/login": "login",
};

/** Parses `location.hash` values such as `#/t/42` or `#/login?error=x`. */
export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#/, "");
  const [path = "", query = ""] = raw.split("?", 2);
  const params = new URLSearchParams(query);
  const ticketMatch = TICKET_PATTERN.exec(path);
  if (ticketMatch !== null) {
    return { name: "ticket", ticketId: Number(ticketMatch[1]), params };
  }
  return { name: PAGE_PATHS[path] ?? "board", ticketId: null, params };
}

/** Builds the hash for a ticket. */
export function ticketHash(id: number): string {
  return `#/t/${id}`;
}

/** The hash for the board. */
export const BOARD_HASH = "#/";

/** The hash for settings. */
export const SETTINGS_HASH = "#/settings";

/** The hash for delivery stats. */
export const STATS_HASH = "#/stats";

/** Navigates by setting `location.hash`. */
export function navigate(hash: string): void {
  window.location.hash = hash;
}
