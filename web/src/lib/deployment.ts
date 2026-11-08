import type { Repo, TicketSummary, User } from "../api/types";

/** Presentation eligibility; the server validates every deployment request. */
export function canRequestDeploy(ticket: TicketSummary, repo: Repo | undefined, user: User | null): boolean {
  return user !== null && ticket.repo_id !== null && repo?.id === ticket.repo_id
    && repo.deploy_mode === "agent" && ticket.lane === "merged"
    && ticket.stage === "done" && ticket.blocked_reason === "";
}
