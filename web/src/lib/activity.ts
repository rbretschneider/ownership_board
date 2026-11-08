import type { TicketSummary } from "../api/types";
import { isCompletedStage } from "./stages";
export function cardActivity(t: TicketSummary, now = Date.now()): { label: string; working: boolean; blocked: boolean; buildServer?: boolean; activeBuild?: boolean; target?: string } {
  if (t.blocked_reason || t.waiting_on.length) return { label: t.blocked_reason ? "Blocked" : "Waiting", working: false, blocked: true };
  if (t.stage === "deploying") return { label: "Deploying", working: hasLiveLease(t, now), blocked: false };
  if (t.stage === "deployed") return { label: "Deployed", working: false, blocked: false };
  if (t.stage === "done") return { label: "Merged", working: false, blocked: false };
  const build = t.stage === "build" && t.work_model?.match(/ · build-server:(queued|paused|preparing|building|testing|verifying):([^·]+) · awaiting Windows$/);
  if (build && build[1] && build[2] && !t.needs_owner) {
    const phase = hasLiveLease(t, now) || ["queued", "paused"].includes(build[1]) ? build[1] : "queued";
    const labels: Record<string, string> = { queued: "Queued", paused: "Paused", preparing: "Preparing", building: "Building", testing: "Testing", verifying: "Checking" };
    const target = build[2].trim();
    return { label: `${labels[phase]} · ${target}`, working: false, blocked: false, buildServer: true, activeBuild: !["queued", "paused"].includes(phase), target };
  }
  if (t.stage === "build" && t.work_model?.endsWith(" · awaiting verification")) return { label: "Verification needed", working: false, blocked: false };
  if (t.stage === "build" && t.work_model?.endsWith(" · awaiting Windows")) return { label: "Awaiting Windows", working: false, blocked: false };
  if (t.needs_owner) return { label: t.open_questions ? "Needs you" : "Awaiting review", working: false, blocked: false };
  if (hasLiveLease(t, now)) {
    const signal = t.work_model?.toLowerCase() ?? "";
    if (signal.includes("preparing workspace")) return { label: "Preparing", working: true, blocked: false };
    if (signal.includes("awaiting review")) return { label: "Awaiting review", working: false, blocked: false };
    if (signal.includes("adversarial review")) return { label: "Reviewing", working: true, blocked: false };
    const labels = { deploying: "Deploying", refine: "Refining", ready: "Starting", design: "Designing", build: "Building", spec_review: "Reviewing", design_review: "Reviewing", pr: "Reviewing" };
    return { label: labels[t.stage], working: true, blocked: false };
  }
  return { label: ["spec_review", "design_review", "pr"].includes(t.stage) ? "Awaiting review" : "Queued", working: false, blocked: false };
}
export function hasLiveLease(t: TicketSummary, now = Date.now()): boolean {
  return t.lease !== null && Date.parse(t.lease.expires_at) > now;
}
export function cardTone(t: TicketSummary, now = Date.now()): string {
  const a = cardActivity(t, now);
  return a.blocked ? " card--blocked" : a.activeBuild ? " card--build-server" : a.working ? " card--working" : !isCompletedStage(t.stage) && t.lane !== "todo" ? " card--idle" : "";
}
