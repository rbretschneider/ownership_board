import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { TicketSummary } from "../../api/types";
import { parseWorkModel, shortTarget } from "../../lib/workModel";
import { TicketCardContent } from "./TicketCardContent";

const story: TicketSummary = {
 id: 9, repo_id: null, kind: "story", title: "A product outcome", lane: "review", stage: "pr",
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
 it("keeps autonomous delivery off the owner's action list", () => {
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
  card = dom({stage: "build", lane: "in_progress", work_model: "gpt-6.1-sol · adversarial review"});
  expect(card.querySelector(".card__top .card__activity-label")?.textContent).toBe("Reviewing");
  expect(card.querySelectorAll(".pixel-loader i")).toHaveLength(9);
  card = dom({stage: "build", work_model: "gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows"});
  expect(card.querySelector(".pixel-loader")).toBeNull();
  expect(card.querySelector(".card__activity")?.className).toContain("badge--build-target");
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
 expect(steps.map((s) => s.getAttribute("aria-label")?.split(":")[0])).toEqual(["Refine and design", "Build", "Windows verification", "Fresh-context adversarial review", "Pull request and checks"]);
 expect(card.querySelector('[aria-current="step"]')?.getAttribute("aria-label")).toBe("Windows verification: current step · 5 Windows runs, 4 failed");
 expect(card.querySelectorAll(".stepper__step--done")).toHaveLength(2);
 expect(steps.map((s) => s.textContent)).toEqual(["Refine", "Build4", "Test5", "Review", "PR"]);
 expect(card.querySelector('[aria-current="step"] .stepper__label')?.textContent).toBe("Test");
 expect(card.querySelectorAll(".stepper__step--retried")).toHaveLength(2);
 // Chevrons close the card: the stepper is its last element.
 expect(card.lastElementChild?.className).toBe("stepper");
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
});
