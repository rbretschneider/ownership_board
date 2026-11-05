import type { PRCheck, PRStatus, PRStatusSnapshot, TicketSummary } from "../api/types";

/** The merge sweep runs every 5 minutes; three missed runs means it stopped. */
export const PR_STATUS_STALE_MS = 15 * 60_000;

export const EMPTY_PR_STATUS: PRStatusSnapshot = { observed_at: null, prs: [] };

export type PRTone = "running" | "owner" | "failed" | "done" | "idle";

export interface PRPill {
  label: string;
  tone: PRTone;
  title: string;
}

const REPAIR_LABELS: Record<Exclude<PRStatus["reason"], "">, string> = {
  conflict: "Conflict · back to build",
  changes_requested: "Changes requested · back to build",
  checks_failed: "CI failed · back to build",
  unresolved: "Unresolved · back to build",
};

function minutes(ms: number): string {
  const m = Math.max(0, Math.floor(ms / 60_000));
  return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`;
}

function isDone(check: PRCheck): boolean {
  return check.status === "COMPLETED";
}

function checkWord(check: PRCheck): string {
  if (!isDone(check)) return check.status === "IN_PROGRESS" ? "running" : "queued";
  return check.conclusion.toLowerCase().replace(/_/g, " ") || "done";
}

/** The status the sweep last recorded for this ticket's PR, if it is fresh and matches. */
export function ticketPR(snapshot: PRStatusSnapshot, ticket: Pick<TicketSummary, "id" | "pr_url">, now: number): PRStatus | undefined {
  if (snapshot.observed_at === null || now - Date.parse(snapshot.observed_at) > PR_STATUS_STALE_MS) return undefined;
  const status = snapshot.prs.find((p) => p.ticket_id === ticket.id);
  if (!status || (ticket.pr_url && ticket.pr_url !== status.url)) return undefined;
  return status;
}

/** One pill that says what a PR in Delivery is waiting for, and who moves next. */
export function prPill(status: PRStatus, observedAt: string | null, now: number): PRPill {
  const total = status.checks.length;
  const done = status.checks.filter(isDone).length;
  const running = status.checks.filter((c) => !isDone(c));
  const startedTimes = running.map((c) => (c.started_at ? Date.parse(c.started_at) : NaN)).filter((t) => !Number.isNaN(t));
  const since = startedTimes.length ? minutes(now - Math.min(...startedTimes)) : "";
  let label: string;
  let tone: PRTone = "idle";
  switch (status.verdict) {
    case "running":
      label = `CI running · ${done}/${total}${since ? ` · ${since}` : ""}`;
      tone = "running";
      break;
    case "no_checks": label = "Waiting for CI"; break;
    case "draft": label = "Draft PR"; break;
    case "checking_mergeable": label = "Checking mergeability"; break;
    case "checks_incomplete":
      label = status.checks.some((c) => c.conclusion === "CANCELLED") ? "CI cancelled · stuck" : "CI incomplete · stuck";
      tone = "failed";
      break;
    case "green_owner": label = "Green · needs your merge"; tone = "owner"; break;
    case "merged": label = "Merged"; tone = "done"; break;
    case "merge_failed": label = "Green · merge failed"; tone = "failed"; break;
    case "closed": label = "PR closed"; tone = "failed"; break;
    case "repair": label = status.reason ? REPAIR_LABELS[status.reason] : "Back to build"; tone = "failed"; break;
  }
  const number = status.url.match(/\/pull\/(\d+)$/)?.[1];
  const lines = [`PR #${number ?? "?"}${status.head ? ` · commit ${status.head.slice(0, 8)}` : ""}`];
  if (total) lines.push("Checks: " + status.checks.map((c) => `${c.name} ${checkWord(c)}`).join(", "));
  if (status.unresolved) lines.push("Published with unresolved problems: never merged automatically; it returns to build for repair.");
  if (status.gl_waived) lines.push("windows-msvc's GL runner check is waived (temporary).");
  if (status.verdict === "running" && status.may_merge && status.acts && !status.unresolved) lines.push("Merges automatically when every check passes.");
  if (!status.acts) lines.push("Not an autopilot ticket and the repo has no merge grant: the sweep only reports on it.");
  if (observedAt) lines.push(`Merge sweep checked ${minutes(now - Date.parse(observedAt))} ago (every 5 minutes).`);
  return { label, tone, title: lines.join("\n") };
}
