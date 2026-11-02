import type { TicketSummary } from "../api/types";
export function cardActivity(t: TicketSummary, now = Date.now()): { label: string; working: boolean; blocked: boolean } {
  if (t.blocked_reason || t.waiting_on.length) return { label: t.blocked_reason ? "Blocked" : "Waiting", working: false, blocked: true };
  if (t.stage === "done") return { label: "Delivered", working: false, blocked: false };
  if (t.stage === "build" && t.work_model?.endsWith(" · awaiting Windows")) return { label: "Awaiting Windows", working: false, blocked: false };
  if (t.needs_owner) return { label: t.open_questions ? "Needs you" : "Awaiting review", working: false, blocked: false };
  if (hasLiveLease(t, now)) {
    const signal = t.work_model?.toLowerCase() ?? "";
    if (signal.includes("awaiting review")) return { label: "Awaiting review", working: false, blocked: false };
    if (signal.includes("adversarial review")) return { label: "Reviewing", working: true, blocked: false };
    const labels = { refine: "Refining", ready: "Starting", design: "Designing", build: "Building", spec_review: "Reviewing", design_review: "Reviewing", pr: "Reviewing" };
    return { label: labels[t.stage], working: true, blocked: false };
  }
  return { label: ["spec_review", "design_review", "pr"].includes(t.stage) ? "Awaiting review" : "Queued", working: false, blocked: false };
}
export function hasLiveLease(t: TicketSummary, now = Date.now()): boolean {
  return t.lease !== null && Date.parse(t.lease.expires_at) > now;
}
export function cardTone(t: TicketSummary, now = Date.now()): string {
  const a = cardActivity(t, now);
  return a.blocked ? " card--blocked" : a.working ? " card--working" : t.stage !== "done" && t.lane !== "todo" ? " card--idle" : "";
}
