import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { TicketSummary } from "../../api/types";
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
describe("owner-facing story status", () => {
 it("shows the worker model instead of its coordinator token", () => {
  expect(render()).toContain("gpt-6.1-sol");
  expect(render()).not.toContain("AI working");
 });
 it("keeps autonomous delivery off the owner's action list", () => {
  expect(render()).toContain("On autopilot");
  expect(render()).not.toContain("Needs you");
  expect(render()).toContain("Delivery checks");
 });
 it("shows real owner blockers", () => {
  expect(render({needs_owner: true, blocked_reason: "Credentials needed"})).toContain("Needs you");
 });
 it("does not invent a model for older agents or show an inactive model", () => {
  expect(render({work_model: ""})).toContain("Agent working");
  expect(render({lease: null})).not.toContain("gpt-6.1-sol");
 });
});

it("places autopilot on the kind row and animates only active work", () => {
 const dom = document.createElement("div");
 dom.innerHTML = render({stage: "ready", lane: "todo", lease: null});
 expect(dom.querySelector(".card__top .card__activity")?.textContent).toBe("Autopilot");
 expect(dom.querySelector(".pixel-loader")).toBeNull();
 dom.innerHTML = render({stage: "build", lane: "in_progress", work_model: "gpt-6.1-sol · adversarial review"});
 expect(dom.querySelector(".card__top .card__activity")?.textContent).toBe("Reviewing");
 expect(dom.querySelectorAll(".pixel-loader i")).toHaveLength(9);
 expect(dom.querySelector(".card__model")?.textContent).toBe("gpt-6.1-sol");
});
it("replaces the step count with a stage stepper showing retries", () => {
 const dom = document.createElement("div");
 dom.innerHTML = render({stage: "build", lane: "in_progress", work_model: "gpt-6.1-sol · build-server:testing:Windows x64 · awaiting Windows",
  attempts: {build: 4, test: 5, test_failures: 4, review: 0, review_fixes: 0, pr: 0, pr_repairs: 0}});
 expect(dom.textContent).not.toContain("steps complete");
 const steps = [...dom.querySelectorAll(".stepper__step")];
 expect(steps.map((s) => s.getAttribute("aria-label")?.split(":")[0])).toEqual(["Refine and design", "Build", "Windows verification", "Fresh-context adversarial review", "Pull request and checks"]);
 expect(dom.querySelector('[aria-current="step"]')?.getAttribute("aria-label")).toBe("Windows verification: current step · 5 Windows runs, 4 failed");
 expect(dom.querySelectorAll(".stepper__step--done")).toHaveLength(2);
 expect(dom.querySelector(".stepper__summary")?.textContent).toBe("Build4Test5");
 expect(dom.querySelector(".stepper__item--current .stepper__label")?.textContent).toBe("Test");
});
it("does not label delivered stories as awaiting delivery checks", () => {
 expect(render({stage: "done", lane: "merged", lease: null})).not.toContain("Delivery checks");
});
it("keeps the responsible model alongside the build target during offload", () => {
 const dom = document.createElement("div");
 dom.innerHTML = render({stage: "build", work_model: "gpt-6.1-sol · build-server:building:Windows x64 · awaiting Windows"});
 expect(dom.querySelector(".card__activity")?.textContent).toBe("Building");
 expect(dom.querySelector(".card__model")?.textContent).toBe("Build server · Windows x64");
 expect(dom.querySelectorAll(".card__model")[1]?.textContent).toBe("gpt-6.1-sol");
 expect(dom.querySelectorAll(".pixel-loader i")).toHaveLength(9);
 dom.innerHTML = render({stage: "build", work_model: "gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows"});
 expect(dom.querySelector(".pixel-loader")).toBeNull();
});
