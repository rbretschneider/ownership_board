import type { RunnerQueued, RunnerSlot, RunnersSnapshot } from "../api/types";

/** The host writes a snapshot every few seconds; older than this means it is not reporting. */
export const RUNNERS_STALE_MS = 45_000;

export const EMPTY_RUNNERS: RunnersSnapshot = { observed_at: null, host: "", enabled: false, parallel_builds: 0, blocked: "", blocked_since: null, slots: [], queue: [] };

/** Why queued work is not starting, in words the owner can act on. */
export const BLOCK_REASONS: Record<Exclude<RunnersSnapshot["blocked"], "">, { short: string; long: string }> = {
  "github-actions": { short: "PC busy: GitHub Actions job", long: "The Windows PC is running a GitHub Actions job (for example the MakerCAD cache warm after a merge). Board jobs start when it finishes." },
  disabled: { short: "paused in Settings", long: "The Windows runner is switched off in Settings." },
  "low-disk": { short: "paused: low disk", long: "The Windows PC is below its free-space reserve, so no new jobs start." },
  "no-settings": { short: "can't reach the board", long: "The Windows PC cannot read the board's settings, so it does not start new jobs." },
  drained: { short: "slots blocked", long: "New Windows jobs are blocked (drained) so the PC stays idle." },
};

export type HostState = "unknown" | "offline" | "paused" | "online";

export function hostState(snapshot: RunnersSnapshot, now: number): HostState {
  if (snapshot.observed_at === null) return "unknown";
  if (now - Date.parse(snapshot.observed_at) > RUNNERS_STALE_MS) return "offline";
  return snapshot.enabled ? "online" : "paused";
}

const PHASE_WORDS: Record<RunnerSlot["phase"], string> = {
  queued: "waiting", preparing: "preparing", building: "building", testing: "testing", verifying: "verifying",
};

/** What a slot is doing, in a few words: the host's step when it sent one. */
export function slotStep(slot: RunnerSlot): string {
  return slot.detail || PHASE_WORDS[slot.phase];
}

/** waiting: the job holds a slot but waits for a shared resource (the desktop, configure). */
export function slotTone(slot: RunnerSlot): "orphan" | "waiting" | "testing" | "building" {
  if (slot.orphan) return "orphan";
  const step = slotStep(slot).toLowerCase();
  if (slot.phase === "queued" || step.includes("waiting")) return "waiting";
  if (slot.phase === "testing" || step.includes("test")) return "testing";
  return "building";
}

/** Whole minutes since an ISO time, or null. */
export function minutesSince(iso: string | null, now: number): number | null {
  if (iso === null) return null;
  const at = Date.parse(iso);
  return Number.isFinite(at) ? Math.max(0, Math.floor((now - at) / 60_000)) : null;
}

export function formatMinutes(minutes: number | null): string {
  if (minutes === null) return "";
  return minutes < 60 ? `${minutes}m` : `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

/** Where a ticket's Windows work is right now, per the host. */
export type TicketRun =
  | { kind: "running"; slot: RunnerSlot }
  | { kind: "queued"; entry: RunnerQueued }
  | { kind: "orphan"; slot: RunnerSlot };

/**
 * A ticket's live run. A current run wins over a queued one, and either wins
 * over an orphan (an old commit still occupying a slot), so the card shows
 * what the ticket is actually waiting on.
 */
export function ticketRun(snapshot: RunnersSnapshot, ticketId: number, now: number): TicketRun | undefined {
  if (hostState(snapshot, now) === "unknown" || hostState(snapshot, now) === "offline") return undefined;
  const running = snapshot.slots.find((s) => s.ticket_id === ticketId && !s.orphan);
  if (running) return { kind: "running", slot: running };
  const queued = snapshot.queue.find((q) => q.ticket_id === ticketId);
  if (queued) return { kind: "queued", entry: queued };
  const orphan = snapshot.slots.find((s) => s.ticket_id === ticketId && s.orphan);
  return orphan ? { kind: "orphan", slot: orphan } : undefined;
}
