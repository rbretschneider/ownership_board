import { describe, expect, it } from "vitest";
import type { StageAttempts } from "../api/types";
import { currentStepIndex, progressSteps } from "./progress";

const none: StageAttempts = { build: 0, test: 0, test_failures: 0, review: 0, review_fixes: 0, pr: 0, pr_repairs: 0 };

describe("currentStepIndex", () => {
  it("maps shaping stages to Refine and review to PR", () => {
    expect(currentStepIndex({ stage: "refine" })).toBe(0);
    expect(currentStepIndex({ stage: "design_review" })).toBe(0);
    expect(currentStepIndex({ stage: "pr" })).toBe(4);
    expect(currentStepIndex({ stage: "done" })).toBe(5);
  });
  it("reads the work-model signal inside build", () => {
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol" })).toBe(1);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · awaiting repair" })).toBe(1);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows" })).toBe(2);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · awaiting review" })).toBe(3);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · adversarial review" })).toBe(3);
  });
});

describe("progressSteps", () => {
  it("marks done, current and upcoming steps", () => {
    const steps = progressSteps({ stage: "build", work_model: "x · adversarial review", attempts: { ...none, build: 1, test: 1, review: 1 } });
    expect(steps.map((s) => s.state)).toEqual(["done", "done", "done", "current", "upcoming"]);
    expect(steps.every((s) => s.count === 0)).toBe(true);
  });
  it("shows counts only on steps that ran more than once", () => {
    const attempts = { ...none, build: 4, test: 5, test_failures: 3, review: 1 };
    const steps = progressSteps({ stage: "build", work_model: "x · build-server:testing:Windows x64 · awaiting Windows", attempts });
    expect(steps.map((s) => s.count)).toEqual([0, 4, 5, 0, 0]);
    expect(steps[2]?.hasRetries).toBe(true);
    expect(steps[2]?.title).toContain("5 Windows runs, 3 failed");
    expect(steps[1]?.title).toContain("4 passes (3 after failed Windows runs)");
  });
  it("marks a passed-over step with nothing recorded as skipped", () => {
    const steps = progressSteps({ stage: "done", attempts: { ...none, build: 1, review: 1, pr: 1 } });
    expect(steps.map((s) => s.state)).toEqual(["done", "done", "skipped", "done", "done"]);
  });
  it("shows no counts and no skipped steps when the server sends no attempts", () => {
    const steps = progressSteps({ stage: "done" });
    expect(steps.map((s) => s.state)).toEqual(["done", "done", "done", "done", "done"]);
    expect(steps.every((s) => s.count === 0)).toBe(true);
  });
});
