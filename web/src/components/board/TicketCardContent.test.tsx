import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { PRStatusContext } from "../../hooks/usePRStatus";
import type { Repo, PRStatusSnapshot, TicketSummary } from "../../api/types";
import { modelFamily, parseWorkModel, shortTarget } from "../../lib/workModel";
import { TicketCardContent } from "./TicketCardContent";

const story: TicketSummary = {
 id: 9, repo_id: null, kind: "story", title: "A product outcome", lane: "in_progress", stage: "pr",
 owner: "agent", needs_owner: false, blocked_reason: "", is_autonomous: true,
 spec_revision: 1, design_revision: 1, pr_url: "https://github.com/acme/app/pull/9", branch: "feature",
 open_questions: 0, waiting_on: [], lease: {holder: "runner-2", expires_at: "2099-01-01T00:00:00Z"},
 work_model: "gpt-6.1-sol", created_at: "", updated_at: "", lane_changed_at: "",
};
function render(overrides: Partial<TicketSummary> = {}) {
 return renderToStaticMarkup(<TicketCardContent ticket={{...story, ...overrides}} stages={[]} repo={undefined} />);
}
function dom(overrides: Partial<TicketSummary> = {}) {
 const root = document.createElement("div");
 root.innerHTML = render(overrides);
 return root;
}
describe("owner-facing story status", () => {
 it("shows the worker model instead of its coordinator token", () => {
  expect(render()).toContain("gpt-6.1-sol");
  expect(render()).not.toContain("AI working");
 });
 it("keeps autonomous work off the owner's action list", () => {
  expect(render()).toContain("On autopilot");
  expect(render()).not.toContain("Needs you");
 });
 it("shows real owner blockers", () => {
  expect(render({needs_owner: true, blocked_reason: "Credentials needed"})).toContain("Credentials needed");
  expect(render({needs_owner: true, is_autonomous: false, lease: null, lane: "todo", stage: "spec_review"})).toContain("Needs you");
 });
 it("does not invent a model for older agents or show an inactive model", () => {
  expect(render({work_model: ""})).toContain("Agent working");
  expect(render({lease: null})).not.toContain("gpt-6.1-sol");
 });
});

describe("card top row", () => {
 it("right-aligns the model and one status pill, and keeps the meta row to the repo", () => {
  const card = dom({stage: "build", lane: "in_progress", work_model: "gpt-6.1-sol · build-server:verifying:Windows x64 · awaiting Windows"});
  const right = card.querySelector(".card__top .card__top-right");
  expect([...(right?.children ?? [])].map((c) => c.className.split(" ")[0])).toEqual(["card__model", "badge"]);
  expect(right?.querySelector(".card__model")?.textContent).toBe("gpt-6.1-sol");
  expect(right?.querySelector(".card__activity-label")?.textContent).toBe("Checking");
  expect(right?.querySelector(".card__activity-target")?.textContent).toBe("Win x64");
  expect(right?.querySelector(".card__activity")?.getAttribute("title")).toBe("Build server · Checking · Windows x64");
  expect(card.querySelector(".card__meta")?.textContent).not.toContain("Build server");
  expect(card.querySelector(".card__meta .card__model")).toBeNull();
  expect(card.textContent).not.toContain("Delivery checks");
 });
 it("places autopilot on the top row and animates only active work", () => {
  let card = dom({stage: "ready", lane: "todo", lease: null});
  expect(card.querySelector(".card__top .card__activity-label")?.textContent).toBe("Autopilot");
  expect(card.querySelector(".pixel-loader")).toBeNull();
  card = dom({stage: "build", lane: "in_progress", work_model: "mystery-model · adversarial review"});
  expect(card.querySelector(".card__top .card__activity-label")?.textContent).toBe("Reviewing");
  expect(card.querySelectorAll(".pixel-loader i")).toHaveLength(9);
  expect(card.querySelector(".miner")).toBeNull();
  card = dom({stage: "build", work_model: "gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows"});
  expect(card.querySelector(".pixel-loader")).toBeNull();
  expect(card.querySelector(".card__activity")?.className).toContain("badge--build-target");
 });
 it("shows a Claude crab or a Codex terminal mining while a model builds", () => {
  let card = dom({stage: "build", lane: "in_progress", work_model: "opus · preparing workspace"});
  expect(card.querySelector(".card__activity .miner")?.getAttribute("data-family")).toBe("claude");
  expect(card.querySelector(".pixel-loader")).toBeNull();
  card = dom({stage: "build", lane: "in_progress", work_model: "gpt-6.1-sol · adversarial review"});
  expect(card.querySelector(".card__activity .miner")?.getAttribute("data-family")).toBe("codex");
  // Not while the Windows build server has it, without a live lease, or outside build.
  expect(dom({stage: "build", lane: "in_progress", work_model: "opus · build-server:testing:Windows x64 · awaiting Windows"}).querySelector(".miner")).toBeNull();
  expect(dom({stage: "build", lane: "in_progress", lease: null, work_model: "opus"}).querySelector(".miner")).toBeNull();
  expect(dom({stage: "design", lane: "in_progress", work_model: "opus"}).querySelector(".miner")).toBeNull();
  expect(dom({work_model: "opus"}).querySelector(".miner")).toBeNull();
 });
 it("names what a waiting ticket waits on in its status pill", () => {
  const card = dom({stage: "ready", lane: "todo", lease: null, waiting_on: [20, 25]});
  expect(card.querySelector(".card__activity-label")?.textContent).toBe("Waiting on #20, #25");
  expect(card.querySelectorAll(".badge--waiting")).toHaveLength(0);
 });
 it("marks an escalated model in orange with rank chevrons and says why", () => {
  const card = dom({stage: "build", lane: "in_progress", work_model: "gpt-6-astra [escalated:17] · build-server:queued:Windows x64 · awaiting Windows"});
  const model = card.querySelector(".card__model");
  expect(model?.className).toContain("card__model--escalated");
  expect(model?.textContent).toBe("gpt-6-astra");
  expect(model?.querySelector(".rank-chevrons")).not.toBeNull();
  expect(model?.getAttribute("title")).toBe("Escalated to gpt-6-astra at high reasoning after 17 rejected code passes");
  // The build-server status still parses with the tag in place.
  expect(card.querySelector(".card__activity-label")?.textContent).toBe("Queued");
  expect(dom().querySelector(".card__model--escalated")).toBeNull();
 });
});

it("replaces the step count with a stage stepper showing retries", () => {
 const card = dom({stage: "build", lane: "in_progress", work_model: "gpt-6.1-sol · build-server:testing:Windows x64 · awaiting Windows",
  attempts: {build: 4, test: 5, test_failures: 4, review: 0, review_fixes: 0, pr: 0, pr_repairs: 0}});
 expect(card.textContent).not.toContain("steps complete");
 const steps = [...card.querySelectorAll(".stepper__step")];
 expect(steps.map((s) => s.getAttribute("aria-label")?.split(":")[0])).toEqual(["Refine and design", "Build and test", "Fresh-context adversarial review", "Pull request and checks"]);
 // A Windows run is part of Build; the live phase is in the status pill.
 expect(card.querySelector('[aria-current="step"]')?.getAttribute("aria-label")).toBe("Build and test: current step · 4 passes (4 after failed Windows runs)");
 expect(card.querySelectorAll(".stepper__step--done")).toHaveLength(1);
 expect(steps.map((s) => s.textContent)).toEqual(["Refine", "Build4", "Review", "PR"]);
 expect(card.querySelectorAll(".stepper__step--retried")).toHaveLength(1);
 expect(card.querySelector('[class*="badge--windows"]')).toBeNull();
 // Chevrons close the card: the stepper is its last element.
 expect(card.lastElementChild?.className).toBe("stepper");
});

it("shows the Windows result as a status once verification is over", () => {
 const passed = dom({stage: "pr", lane: "in_progress", attempts: {build: 2, test: 2, test_failures: 1, review: 1, review_fixes: 0, pr: 1, pr_repairs: 0}});
 const chip = passed.querySelector(".badge--windows-passed");
 expect(chip?.textContent).toBe("Windows passed");
 expect(chip?.getAttribute("title")).toBe("Windows verification passed · 2 Windows runs, 1 failed");
 // A repo without Windows checks shows nothing.
 const none = dom({stage: "pr", lane: "in_progress", attempts: {build: 1, test: 0, test_failures: 0, review: 1, review_fixes: 0, pr: 1, pr_repairs: 0}});
 expect(none.querySelector('[class*="badge--windows"]')).toBeNull();
});

describe("work model signal", () => {
 it("reads the model name and an escalation tag", () => {
  expect(parseWorkModel("gpt-6.1-sol · adversarial review")).toEqual({name: "gpt-6.1-sol"});
  expect(parseWorkModel("gpt-6-astra [escalated:17] · awaiting review")).toEqual({name: "gpt-6-astra", escalatedAfter: 17});
  expect(parseWorkModel("")).toBeUndefined();
  expect(parseWorkModel(undefined)).toBeUndefined();
  expect(shortTarget("Windows x64")).toBe("Win x64");
  expect(shortTarget("Linux arm64")).toBe("Linux arm64");
 });
 it("tells Claude models from Codex models", () => {
  for (const name of ["opus", "sonnet", "haiku", "fable", "Claude Opus", "claude-opus-5-5"]) expect(modelFamily(name)).toBe("claude");
  for (const name of ["gpt-6.1-sol", "gpt-6-astra", "Codex (model unavailable)", "o4-mini"]) expect(modelFamily(name)).toBe("codex");
  expect(modelFamily("mystery-model")).toBeUndefined();
  expect(modelFamily(undefined)).toBeUndefined();
 });
});

it("marks scheduled reviews independently of work status", () => {
  for (const stage of ["build", "done"] as const) {
    const card = dom({ is_health_check: true, stage });
    expect(card.querySelectorAll(".badge--health-check")).toHaveLength(1);
    expect(card.querySelector(".badge--health-check")?.textContent).toBe("Health check");
    expect(card.querySelector('.badge--health-check svg[aria-hidden="true"]')).not.toBeNull();
  }
  expect(dom({ is_health_check: false }).textContent).not.toContain("Health check");
  expect(dom().textContent).not.toContain("Health check");
});

const prNow = Date.parse("2026-11-08T12:00:00Z");
const prSnapshot: PRStatusSnapshot = { observed_at: new Date(prNow).toISOString(), prs: [{
 ticket_id: story.id, url: story.pr_url, verdict: "green_owner", reason: "", may_merge: false, acts: false,
 gl_waived: false, draft: false, mergeable: "MERGEABLE", review: "", head: "abc123", unresolved: false, manual_check: false, checks: [],
}] };
function prDom(overrides: Partial<TicketSummary> = {}, snapshot = prSnapshot) {
 const host = document.createElement("div");
 host.innerHTML = renderToStaticMarkup(<PRStatusContext.Provider value={{snapshot, now: prNow}}><TicketCardContent ticket={{...story, ...overrides}} stages={[]} repo={undefined} now={prNow} /></PRStatusContext.Provider>);
 return host;
}
it.each([true, false])("shows a PR pill, owner badge and current PR step together (autonomous=%s)", autonomous => {
 const card = prDom({needs_owner: true, is_autonomous: autonomous});
 expect(card.querySelector(".badge--pr-owner")?.textContent).toBe("Green · needs your merge");
 expect(card.querySelectorAll(".badge--needs")).toHaveLength(1);
 expect(card.querySelector(".badge--needs")?.textContent).toBe("Needs you");
 expect(card.querySelector('[aria-current="step"]')?.textContent).toBe("PR");
 expect(prDom({needs_owner: false, is_autonomous: autonomous}).querySelector(".badge--needs")).toBeNull();
});
it("keeps blockers ahead of PR status and falls back for stale or mismatched snapshots", () => {
 expect(prDom({blocked_reason: "Access needed"}).querySelector(".badge--pr")).toBeNull();
 expect(prDom({blocked_reason: "Access needed"}).querySelector(".badge--blocked")?.getAttribute("title")).toBe("Blocked: Access needed");
 for (const snapshot of [{...prSnapshot, observed_at: "2000-01-01"}, {...prSnapshot, prs: [{...prSnapshot.prs[0]!, url: "https://github.com/acme/app/pull/99"}]}, {observed_at: null, prs: []}]) {
  expect(prDom({}, snapshot).querySelector(".badge--pr")).toBeNull();
  expect(prDom({}, snapshot).querySelector(".stepper")).not.toBeNull();
 }
});

it("shows deployment waits only for agent deployment and hides completed worker activity", () => {
 for (const mode of ["off", "agent"] as const) {
  const html = renderToStaticMarkup(<TicketCardContent ticket={{...story, lane: "merged", stage: "done"}} stages={[]} repo={{deploy_mode: mode} as Repo} />);
  expect(html.includes("Awaiting deploy")).toBe(mode === "agent");
 }
 expect(render({lane: "merged", stage: "done"})).not.toContain("Awaiting deploy");
 const html = render({lane: "deployed", stage: "deployed"});
 expect(html).not.toContain("card__activity");
 expect(html).not.toContain("card__model");
 expect(html).not.toContain('aria-current=');
});

it("shows deployment, animation and the live holder while suppressing stale models", () => {
 const card = dom({stage: "deploying", lane: "merged", work_model: "model · adversarial review"});
 expect(card.querySelector(".card__activity-label")?.textContent).toBe("Deploying");
 expect(card.querySelector(".pixel-loader")).not.toBeNull();
 expect(card.querySelector(".card__lease-holder")?.textContent).toBe("runner-2");
 expect(card.querySelector(".card__model")).toBeNull();
 for (const lease of [null, {holder: "old-worker", expires_at: "2000-01-01"}]) {
  const idle = dom({stage: "deploying", lane: "merged", lease});
  expect(idle.querySelector(".card__activity-label")?.textContent).toBe("Deploying");
  expect(idle.querySelector(".pixel-loader")).toBeNull();
  expect(idle.querySelector(".card__lease-holder")).toBeNull();
 }
 const blocked = dom({stage: "done", lane: "merged", blocked_reason: "Deployment failed"});
 expect(blocked.querySelector(".badge--blocked")?.textContent).toContain("Blocked");
 expect(blocked.querySelector(".badge--blocked")?.getAttribute("title")).toContain("Deployment failed");
});

describe("token usage", () => {
 const usage = {calls: 4, input_tokens: 300, cache_write_tokens: 40_000, cache_read_tokens: 2_000_000, output_tokens: 9_700, cost_usd: 1.25, weighted_tokens: 250_000};
 it("shows nothing until usage is reported", () => {
  expect(dom().querySelector(".badge--usage")).toBeNull();
  expect(dom({usage: {...usage, calls: 0, weighted_tokens: 0}}).querySelector(".badge--usage")).toBeNull();
 });
 it("shows weighted tokens quietly, with the breakdown on hover", () => {
  const badge = dom({usage}).querySelector(".card__meta .badge--usage");
  expect(badge?.textContent).toBe("250k tok");
  expect(badge?.classList.contains("badge--usage-heavy")).toBe(false);
  expect(badge?.getAttribute("title")).toBe("250k tokens over 4 model calls · 50k new input and output · 2M cache reads, counted at a tenth · Claude list price $1.25");
 });
 it("flags a runaway ticket in bold red", () => {
  const badge = dom({usage: {...usage, weighted_tokens: 14_400_000}}).querySelector(".badge--usage");
  expect(badge?.textContent).toBe("14.4M tok");
  expect(badge?.classList.contains("badge--usage-heavy")).toBe(true);
  expect(badge?.getAttribute("title")).toContain("far more than a tightly scoped story");
 });
});
