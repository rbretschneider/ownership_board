import type { BuildClock } from "../api/types";

/** "45s", "12m", "1h 05m": minute resolution once past a minute. */
export function formatElapsed(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  return `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, "0")}m`;
}

/**
 * How long the ticket's build job has run, or waited when it has not started.
 * Undefined when there is no open job or its times cannot be read.
 */
export function buildClockText(build: BuildClock | null | undefined, now: number): { text: string; title: string } | undefined {
  if (!build) return undefined;
  const queued = Date.parse(build.queued_at);
  const started = build.started_at ? Date.parse(build.started_at) : NaN;
  if (build.status === "running" && Number.isFinite(started)) {
    const waited = Number.isFinite(queued) ? ` after waiting ${formatElapsed(started - queued)}` : "";
    return { text: formatElapsed(now - started), title: `Build running for ${formatElapsed(now - started)}${waited}` };
  }
  if (Number.isFinite(queued)) {
    return { text: formatElapsed(now - queued), title: `Build queued for ${formatElapsed(now - queued)}` };
  }
  return undefined;
}
