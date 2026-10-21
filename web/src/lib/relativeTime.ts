const SECOND_MS = 1000;
const MINUTE_S = 60;
const HOUR_S = 60 * MINUTE_S;
const DAY_S = 24 * HOUR_S;
const WEEK_S = 7 * DAY_S;
const JUST_NOW_S = 45;
const MONTH_NAMES = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/**
 * Formats an ISO timestamp relative to `now`: "just now", "5m ago", "3h ago",
 * "2d ago", then a short date ("Mar 4", or "Mar 4, 2025" in another year).
 * Future times (clock skew) read as "just now".
 */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) {
    return "";
  }
  const seconds = Math.floor((now.getTime() - then.getTime()) / SECOND_MS);
  if (seconds < JUST_NOW_S) {
    return "just now";
  }
  if (seconds < HOUR_S) {
    return `${Math.max(1, Math.round(seconds / MINUTE_S))}m ago`;
  }
  if (seconds < DAY_S) {
    return `${Math.floor(seconds / HOUR_S)}h ago`;
  }
  if (seconds < WEEK_S) {
    return `${Math.floor(seconds / DAY_S)}d ago`;
  }
  return shortDate(then, now);
}

function shortDate(date: Date, now: Date): string {
  const label = `${MONTH_NAMES[date.getMonth()]} ${date.getDate()}`;
  return date.getFullYear() === now.getFullYear() ? label : `${label}, ${date.getFullYear()}`;
}
