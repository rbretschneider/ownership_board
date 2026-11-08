import type { Repo, StageAttempts, TicketSummary } from "../api/types";

/** Test fixtures for the flow view, shaped like a live GET /board answer. */

export const NOW = Date.parse("2026-03-10T12:00:00Z");

const NO_ATTEMPTS: StageAttempts = { build: 0, test: 0, test_failures: 0, review: 0, review_fixes: 0, pr: 0, pr_repairs: 0 };

/** A ticket summary with defaults for everything a test does not care about. */
export function ticket(id: number, overrides: Partial<TicketSummary> = {}): TicketSummary {
  return {
    id,
    repo_id: 1,
    kind: "story",
    title: `Story ${id}`,
    lane: "todo",
    stage: "ready",
    owner: "agent",
    needs_owner: false,
    is_autonomous: true,
    blocked_reason: "",
    spec_revision: 1,
    design_revision: 0,
    pr_url: "",
    branch: "",
    open_questions: 0,
    waiting_on: [],
    work_model: "",
    lease: null,
    created_at: "2026-03-01T00:00:00Z",
    updated_at: "2026-03-01T00:00:00Z",
    lane_changed_at: "2026-03-01T00:00:00Z",
    attempts: NO_ATTEMPTS,
    ...overrides,
  };
}

/** A live lease, so the ticket reads as being worked. */
export const LIVE_LEASE = { holder: "worker", expires_at: "2026-03-10T13:00:00Z" };

/** A repo with defaults. */
export function repo(id: number, name: string): Repo {
  return {
    id, provider: "manual", full_name: `example/${name}`, name, url: "", description: "", color: "", default_branch: "main",
    is_private: false, is_archived: false, synced_at: null, missing_since: null, treat_as_public: false, visibility: "public",
    require_spec_approval: false, require_design_approval: false, allow_agent_merge: false,
    health_check_every: 8, health_checked_through_ticket: null, health_checked_at: null,
  };
}

/** In build on the Windows PC, paused, after many failed runs: the live "stuck behind" case. */
export function pausedOnWindows(id: number, overrides: Partial<TicketSummary> = {}): TicketSummary {
  return ticket(id, {
    lane: "in_progress",
    stage: "build",
    work_model: "model-a · build-server:paused:Windows x64 · awaiting Windows",
    attempts: { ...NO_ATTEMPTS, build: 26, test: 28, test_failures: 25 },
    ...overrides,
  });
}

/**
 * A board like the live one: repo 1 has a paused root (#1) gating a chain ten
 * hand-offs deep that fans in to a milestone (#40), plus a busy build; repo 2
 * has a short chain; repo 3 only merged work. ~90 tickets in all.
 */
export function liveLikeBoard(): { tickets: TicketSummary[]; repos: Repo[] } {
  const tickets: TicketSummary[] = [pausedOnWindows(1)];
  tickets.push(ticket(2, { lane: "in_progress", stage: "build", work_model: "model-b", lease: LIVE_LEASE }));
  // Chain levels 1..10 behind #1, three tickets per level, each waiting on one or two of the level before.
  let id = 10;
  let previous = [1];
  for (let level = 1; level <= 10; level++) {
    const current: number[] = [];
    for (let k = 0; k < 3; k++) {
      const deps = [previous[k % previous.length] as number];
      if (previous.length > 1 && k === 2) deps.push(previous[0] as number);
      tickets.push(ticket(id, { waiting_on: [...new Set(deps)] }));
      current.push(id++);
    }
    previous = current;
  }
  tickets.push(ticket(40, { waiting_on: [...previous, 2] }));
  // Repo 2: in Windows verification, with a short chain behind it.
  tickets.push(ticket(50, { repo_id: 2, lane: "in_progress", stage: "build", work_model: "model-a · build-server:verifying:Windows x64 · awaiting Windows", lease: LIVE_LEASE }));
  tickets.push(ticket(51, { repo_id: 2, waiting_on: [50] }));
  tickets.push(ticket(52, { repo_id: 2, waiting_on: [51] }));
  // Merged work across repos, some today.
  for (let m = 0; m < 45; m++) {
    tickets.push(ticket(100 + m, {
      repo_id: 1 + (m % 3),
      lane: "merged",
      stage: "done",
      lane_changed_at: new Date(NOW - m * 3 * 60 * 60 * 1000).toISOString(),
    }));
  }
  return { tickets, repos: [repo(1, "alpha"), repo(2, "beta"), repo(3, "gamma")] };
}
