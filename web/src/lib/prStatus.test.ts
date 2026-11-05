import { describe, expect, it } from "vitest";
import type { PRStatus, PRStatusSnapshot } from "../api/types";
import { prPill, ticketPR } from "./prStatus";

const NOW = Date.parse("2026-11-05T21:00:00Z");
const base: PRStatus = {
  ticket_id: 92, url: "https://github.com/acme/mix/pull/25", verdict: "running", reason: "", may_merge: true, acts: true,
  gl_waived: false, draft: false, mergeable: "MERGEABLE", review: "", head: "9c7c3cf829b4", unresolved: false,
  checks: [
    { name: "build", status: "IN_PROGRESS", conclusion: "", started_at: "2026-11-05T20:48:00Z", completed_at: null },
    { name: "lint", status: "COMPLETED", conclusion: "SUCCESS", started_at: "2026-11-05T20:47:00Z", completed_at: "2026-11-05T20:49:00Z" },
  ],
};
const snapshot = (prs: PRStatus[], observed = "2026-11-05T20:58:00Z"): PRStatusSnapshot => ({ observed_at: observed, prs });

describe("PR pill", () => {
  it("says CI is running, how far along and for how long, and lists each check", () => {
    const pill = prPill(base, "2026-11-05T20:58:00Z", NOW);
    expect(pill).toMatchObject({ label: "CI running · 1/2 · 12m", tone: "running" });
    expect(pill.title).toContain("PR #25 · commit 9c7c3cf8");
    expect(pill.title).toContain("build running, lint success");
    expect(pill.title).toContain("Merges automatically when every check passes.");
    expect(pill.title).toContain("checked 2m ago");
  });

  it("names the owner's move and every way back to build", () => {
    expect(prPill({ ...base, verdict: "green_owner" }, null, NOW)).toMatchObject({ label: "Green · needs your merge", tone: "owner" });
    expect(prPill({ ...base, verdict: "repair", reason: "checks_failed" }, null, NOW).label).toBe("CI failed · back to build");
    expect(prPill({ ...base, verdict: "repair", reason: "conflict" }, null, NOW).label).toBe("Conflict · back to build");
    expect(prPill({ ...base, verdict: "repair", reason: "unresolved", unresolved: true }, null, NOW).title).toContain("never merged automatically");
    expect(prPill({ ...base, verdict: "no_checks", checks: [] }, null, NOW).label).toBe("Waiting for CI");
    const cancelled = { ...base, verdict: "checks_incomplete" as const, checks: [{ ...base.checks[0]!, status: "COMPLETED", conclusion: "CANCELLED" }] };
    expect(prPill(cancelled, null, NOW)).toMatchObject({ label: "CI cancelled · stuck", tone: "failed" });
    expect(prPill({ ...base, acts: false, may_merge: false, verdict: "green_owner" }, null, NOW).title).toContain("only reports on it");
  });

  it("ignores a stale sweep, another PR and other tickets", () => {
    const ticket = { id: 92, pr_url: "https://github.com/acme/mix/pull/25" };
    expect(ticketPR(snapshot([base]), ticket, NOW)).toBe(base);
    expect(ticketPR(snapshot([base], "2026-11-05T20:40:00Z"), ticket, NOW)).toBeUndefined();
    expect(ticketPR(snapshot([base]), { ...ticket, pr_url: "https://github.com/acme/mix/pull/26" }, NOW)).toBeUndefined();
    expect(ticketPR(snapshot([base]), { id: 7, pr_url: "" }, NOW)).toBeUndefined();
    expect(ticketPR({ observed_at: null, prs: [base] }, ticket, NOW)).toBeUndefined();
  });
});
