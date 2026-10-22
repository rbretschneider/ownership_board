import type { LaneId, Owner } from "../api/types";

/** Narrowest a desktop lane column may get. */
export const COLUMN_MIN_WIDTH_PX = 230;

/** Gap between desktop lane columns. */
export const COLUMN_GAP_PX = 12;

/** Padding around the desktop columns, per side. */
export const COLUMNS_PADDING_PX = 16;

/**
 * Viewport width at which the board switches from one lane to four columns.
 * Four columns at their minimum width fit below this, so there is no
 * horizontal scroll (layout.css `.columns` uses the same numbers).
 */
export const DESKTOP_MIN_WIDTH_PX = 1000;

/** Media query matching the desktop layout. */
export const DESKTOP_MEDIA_QUERY = `(min-width: ${DESKTOP_MIN_WIDTH_PX}px)`;

/** Minimum password length accepted by the setup wizard. */
export const MIN_PASSWORD_LENGTH = 12;

/** Delay before refetching after a live event, so bursts coalesce. */
export const REFETCH_DEBOUNCE_MS = 250;

/** How long toasts stay on screen. */
export const TOAST_DURATION_MS = 4000;

/** Lane the mobile view falls back to. */
export const DEFAULT_LANE: LaneId = "todo";

/** Short lane names for the mobile lane strip. */
export const LANE_SHORT_LABELS: Record<LaneId, string> = {
  todo: "Todo",
  in_progress: "Doing",
  review: "Review",
  merged: "Merged",
};

/** One-line hints shown in empty lanes. */
export const LANE_EMPTY_HINTS: Record<LaneId, string> = {
  todo: "Drop an idea here — the AI will refine it.",
  in_progress: "Nothing being designed or built right now.",
  review: "No pull requests waiting for review.",
  merged: "Merged work lands here.",
};

/** Text labels for ticket owners, so ownership is never conveyed by color alone. */
export const OWNER_LABELS: Record<Owner, string> = {
  agent: "AI",
  human: "You",
  nobody: "—",
};

/** Owner line in the ticket sheet. */
export const OWNER_MOVE_LABELS: Record<Owner, string> = {
  agent: "AI's move",
  human: "Your move",
  nobody: "Done",
};

/** Named color presets for repositories. */
export const REPO_COLORS: readonly { name: string; value: string }[] = [
  { name: "Indigo", value: "#7c8cff" },
  { name: "Sky", value: "#4fb3e8" },
  { name: "Teal", value: "#3cc7a8" },
  { name: "Lime", value: "#9ccc4a" },
  { name: "Amber", value: "#e8b24f" },
  { name: "Coral", value: "#f07c6c" },
  { name: "Rose", value: "#e46fa8" },
  { name: "Slate", value: "#9aa3b2" },
];

/** Label for tickets without a repository. */
export const NO_REPO_LABEL = "No repo — let the AI figure it out";

/** Readable messages for `/#/login?error=<code>` after an SSO callback. */
export const SSO_ERROR_MESSAGES: Record<string, string> = {
  sso_not_linked: "No account is linked to this SSO identity. Sign in with your password, then link SSO in Settings.",
  sso_denied: "Sign-in was cancelled or denied at the identity provider.",
  sso_failed: "Single sign-on failed. Try again, or sign in with your password.",
};

/** SSO link start URL (full navigation, so the IdP redirect works). */
export const OIDC_START_URL = "/api/v1/auth/oidc/start";

/** SSO link URL used from Settings. */
export const OIDC_LINK_URL = "/api/v1/auth/oidc/start?link=1";

/** Server-sent events endpoint. */
export const EVENT_STREAM_URL = "/api/v1/events/stream";
