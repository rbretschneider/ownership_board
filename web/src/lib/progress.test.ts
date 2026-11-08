import { describe, expect, it } from "vitest";
import type { StageAttempts } from "../api/types";
import { currentStepIndex, onWindows, progressSteps, windowsStatus } from "./progress";

const none: StageAttempts = { build: 0, test: 0, test_failures: 0, review: 0, review_fixes: 0, pr: 0, pr_repairs: 0 };
const ON_WINDOWS = "gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows";

describe("currentStepIndex", () => {
  it("maps shaping stages to Refine and review to PR", () => {
    expect(currentStepIndex({ stage: "refine" })).toBe(0);
    expect(currentStepIndex({ stage: "design_review" })).toBe(0);
    expect(currentStepIndex({ stage: "pr" })).toBe(3);
    expect(currentStepIndex({ stage: "done" })).toBe(4);
  });
  it("reads the work-model signal inside build; Windows verification is part of Build", () => {
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol" })).toBe(1);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · awaiting repair" })).toBe(1);
    expect(currentStepIndex({ stage: "build", work_model: ON_WINDOWS })).toBe(1);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · awaiting verification" })).toBe(1);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · awaiting review" })).toBe(2);
    expect(currentStepIndex({ stage: "build", work_model: "gpt-6.1-sol · adversarial review" })).toBe(2);
  });
  it("knows when a build is on Windows", () => {
    expect(onWindows({ stage: "build", work_model: ON_WINDOWS })).toBe(true);
    expect(onWindows({ stage: "build", work_model: "gpt-6.1-sol · awaiting Windows" })).toBe(true);
    expect(onWindows({ stage: "build", work_model: "gpt-6.1-sol" })).toBe(false);
    expect(onWindows({ stage: "pr", work_model: ON_WINDOWS })).toBe(false);
  });
});

describe("progressSteps", () => {
  it("has no Test step: Refine, Build, Review, PR", () => {
    expect(progressSteps({ stage: "refine" }).map((s) => s.label)).toEqual(["Refine", "Build", "Review", "PR"]);
  });
  it("marks done, current and upcoming steps", () => {
    const steps = progressSteps({ stage: "build", work_model: "x · adversarial review", attempts: { ...none, build: 1, test: 1, review: 1 } });
    expect(steps.map((s) => s.state)).toEqual(["done", "done", "current", "upcoming"]);
    expect(steps.every((s) => s.count === 0)).toBe(true);
  });
  it("keeps a Windows run on the Build step and counts its retries there", () => {
    const attempts = { ...none, build: 4, test: 5, test_failures: 3, review: 1 };
    const steps = progressSteps({ stage: "build", work_model: "x · build-server:testing:Windows x64 · awaiting Windows", attempts });
    expect(steps.map((s) => s.state)).toEqual(["done", "current", "upcoming", "upcoming"]);
    expect(steps.map((s) => s.count)).toEqual([0, 4, 0, 0]);
    expect(steps[1]?.title).toContain("4 passes (3 after failed Windows runs)");
  });
  it("never marks a done repo without Windows checks as skipping a step", () => {
    const steps = progressSteps({ stage: "done", attempts: { ...none, build: 1, review: 1, pr: 1 } });
    expect(steps.map((s) => s.state)).toEqual(["done", "done", "done", "done"]);
  });
  it("marks a passed-over review with nothing recorded as skipped", () => {
    const steps = progressSteps({ stage: "done", attempts: { ...none, build: 1, pr: 1 } });
    expect(steps.map((s) => s.state)).toEqual(["done", "done", "skipped", "done"]);
  });
  it("shows no counts and no skipped steps when the server sends no attempts", () => {
    const steps = progressSteps({ stage: "done" });
    expect(steps.map((s) => s.state)).toEqual(["done", "done", "done", "done"]);
    expect(steps.every((s) => s.count === 0)).toBe(true);
  });
});

describe("windowsStatus", () => {
  it("is absent when no Windows run happened or one is under way", () => {
    expect(windowsStatus({ stage: "pr", attempts: none })).toBeUndefined();
    expect(windowsStatus({ stage: "pr" })).toBeUndefined();
    expect(windowsStatus({ stage: "build", work_model: ON_WINDOWS, attempts: { ...none, test: 2, test_failures: 1 } })).toBeUndefined();
  });
  it("says the latest run passed when there are more runs than failures", () => {
    expect(windowsStatus({ stage: "pr", attempts: { ...none, test: 3, test_failures: 2 } })).toEqual({
      passed: true, label: "Windows passed", title: "Windows verification passed · 3 Windows runs, 2 failed",
    });
  });
  it("says it failed when every run failed", () => {
    expect(windowsStatus({ stage: "build", work_model: "x · awaiting repair", attempts: { ...none, test: 1, test_failures: 1 } })).toMatchObject({
      passed: false, label: "Windows failed",
    });
  });
});

it("keeps every delivery step completed after deployment", () => {
 expect(currentStepIndex({stage: "deployed"})).toBe(4);
 expect(progressSteps({stage: "deployed"})).toEqual(progressSteps({stage: "done"}));
 expect(progressSteps({stage: "deployed"}).every(s => s.state === "done")).toBe(true);
});
