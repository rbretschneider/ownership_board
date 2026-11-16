import type { Team, TeamTicket, TicketDetail } from "../../api/types";
import { prTicket } from "../board/boardFixture";

/** A fixed clock for Team view tests. */
export const TEAM_NOW = Date.parse("2026-11-16T12:00:00Z");

const minutesAgo = (minutes: number) => new Date(TEAM_NOW - minutes * 60_000).toISOString();

function inFlight(id: number, title: string, overrides: Partial<TeamTicket>): TeamTicket {
  return { ...prTicket, id, title, stage: "build", needs_owner: false, owner: "agent", holder: "", ...overrides };
}

/**
 * Two runners (one working on #12, one idle) and a busy Windows host: #12 on
 * slot 1, #17 on slot 2 with no runner, #13 queued. #14 to #16 exercise the
 * activity lines (blocked, waiting on a dependency, a PR with CI running).
 * The manager is idle; its newest session is structured, the older one not.
 */
export function teamFixture(): Team {
  return {
    observed_at: new Date(TEAM_NOW).toISOString(),
    manager: {
      enabled: true, build_cap: 5, stall_minutes: 120, daily_budget_usd: 10,
      today: { day: "2026-11-16", sessions: 3, cost_usd: 1.94 }, remaining_usd: 8.06,
      open_flags: 2, latest_summary: "MANAGER SUMMARY\nHEADLINE: Directed #12 to fix its brand-scan test.\n- #12 cycling: why: x | action: y | expect: z",
      latest_headline: "Directed #12 to fix its brand-scan test.", latest_at: minutesAgo(20),
      activity: { state: "idle", kind: "", ticket_ids: [], host: "", started_at: null },
      sessions: [
        {
          at: minutesAgo(20), model: "sonnet", cost_usd: 0.1, headline: "Directed #12 to fix its brand-scan test.", structured: true,
          summary: "MANAGER SUMMARY\nHEADLINE: Directed #12 to fix its brand-scan test.\n\n### #12 Release pipeline\n- **Found:** The brand scan fails on a stale fixture.\n- **Did:** Commented the fix.\n- **Next:** The next run passes.\n\n### Owner\n- Nothing.",
        },
        {
          at: minutesAgo(80), model: "sonnet", cost_usd: 0.16, headline: "Held #13 for a scope split.", structured: false,
          summary: "Held #13 for a scope split. why: too large | action: parked | expect: a follow-up ticket",
        },
      ],
    },
    runners: [
      { name: "@worker/7/codex_1", state: "working", model: "gpt-test", doing: "repairing the release workflow", ticket_id: 12, last_seen: minutesAgo(2), profile_version: "" },
      { name: "@worker/7/claude_2", state: "idle", model: "opus-test", doing: "No ticket held", ticket_id: null, last_seen: minutesAgo(90), profile_version: "" },
    ],
    tickets: [
      inFlight(14, "Installer", { blocked_reason: "Needs the owner: signing certificate expired\nDetails follow" }),
      inFlight(15, "Properties panel", { stage: "ready", lane: "todo", waiting_on: [12] }),
      inFlight(12, "Release pipeline", {
        holder: "@worker/7/codex_1",
        usage: { calls: 4, input_tokens: 100_000, cache_write_tokens: 0, cache_read_tokens: 0, output_tokens: 20_000, cost_usd: 1.2, weighted_tokens: 1_900_000 },
        attempts: { build: 3, test: 2, test_failures: 1, review: 1, review_fixes: 0, pr: 0, pr_repairs: 0 },
        build: { status: "running", queued_at: minutesAgo(130), started_at: minutesAgo(96) } }),
      inFlight(13, "Brand scan", { stage: "pr", build: { status: "queued", queued_at: minutesAgo(66), started_at: null } }),
      inFlight(17, "Threading pool", { build: { status: "running", queued_at: minutesAgo(50), started_at: minutesAgo(40) } }),
      inFlight(16, "Threading", { stage: "pr", pr: { url: "https://github.com/acme/app/pull/16", verdict: "running", reason: "", checks_total: 8, checks_done: 7, checks_failed: 0 } }),
    ],
    build_hosts: [{
      id: 1, name: "builder-1", os: "windows", online: true, enabled: true, max_parallel: 2,
      running: [
        { ticket_id: 12, repo: "acme/app", status: "running", phase: "testing", detail: "", started_at: minutesAgo(96), attempt: 1 },
        { ticket_id: 17, repo: "acme/app", status: "running", phase: "building", detail: "", started_at: minutesAgo(40), attempt: 1 },
      ],
      queued: [{ ticket_id: 13, repo: "acme/app", queued_at: minutesAgo(66) }],
    }],
    queued_by_os: { windows: 1 },
    ci_runners: {
      observed_at: minutesAgo(2),
      runners: [
        { repo: "acme/app", name: "linux-1", status: "online", busy: true, labels: ["self-hosted", "linux", "X64"] },
        { repo: "acme/app", name: "linux-2", status: "online", busy: false, labels: ["self-hosted", "linux", "X64"] },
        { repo: "acme/app", name: "win-old", status: "offline", busy: false, labels: ["self-hosted", "windows"] },
      ],
    },
  };
}

/** Ticket #12 as GET /tickets/12 returns it, with a short history. */
export function ticketDetailFixture(): TicketDetail {
  const ticket = teamFixture().tickets.find((t) => t.id === 12)!;
  return {
    ...ticket, body: "", spec: "", design: "", approved_spec_revision: 1, approved_design_revision: 1, created_by: "Owner",
    gates: { spec: false, design: false }, questions: [], work_links: [], decisions: [], relations: [],
    timeline: [
      { id: 1, type: "claimed", at: minutesAgo(40), actor: { kind: "agent", name: "@worker/7/codex_1" }, body: "", data: { action: "implement" } },
      { id: 2, type: "edited", at: minutesAgo(30), actor: { kind: "agent", name: "@worker/7/codex_1" }, body: "", data: { fields: ["working model"], model: "gpt-test · build-server:queued:Windows x64 · awaiting Windows" } },
      { id: 3, type: "comment", at: minutesAgo(25), actor: { kind: "agent", name: "@worker/7/codex_1" }, body: "Windows verification queued for commit abc1234\nmore", data: {} },
    ],
  } as unknown as TicketDetail;
}
