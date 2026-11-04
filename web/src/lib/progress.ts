import type { StageAttempts, TicketSummary } from "../api/types";

export type StepId = "refine" | "build" | "test" | "review" | "pr";

export const PROGRESS_STEPS: { id: StepId; label: string; about: string }[] = [
  { id: "refine", label: "Refine", about: "Refine and design" },
  { id: "build", label: "Build", about: "Build" },
  { id: "test", label: "Test", about: "Windows verification" },
  { id: "review", label: "Review", about: "Fresh-context adversarial review" },
  { id: "pr", label: "PR", about: "Pull request and checks" },
];

export type StepState = "done" | "current" | "upcoming" | "skipped";

export interface Step {
  id: StepId;
  label: string;
  state: StepState;
  /** Passes through this step; 0 unless the step ran more than once. */
  count: number;
  /** The step bounced back at least once (a failure or requested fixes). */
  hasRetries: boolean;
  title: string;
}

/**
 * Which step the ticket is on (0–4), or 5 once delivered. Within build, the
 * work-model signal says whether the code is being written, verified on
 * Windows or reviewed.
 */
export function currentStepIndex(ticket: Pick<TicketSummary, "stage" | "work_model">): number {
  switch (ticket.stage) {
    case "build": {
      const signal = ticket.work_model?.split(" · ").slice(1).join(" · ") ?? "";
      if (signal.startsWith("build-server:") || signal === "awaiting Windows" || signal === "awaiting verification") return 2;
      if (signal.includes("adversarial review") || signal === "awaiting review") return 3;
      return 1;
    }
    case "pr":
      return 4;
    case "done":
      return 5;
    default:
      return 0;
  }
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : word.endsWith("s") ? "es" : "s"}`;
}

function stepDetail(id: StepId, a: StageAttempts): { count: number; hasRetries: boolean; detail: string } {
  switch (id) {
    case "build": {
      const why = [
        a.test_failures > 0 && `${a.test_failures} after failed Windows runs`,
        a.review_fixes > 0 && `${a.review_fixes} after review fixes`,
        a.pr_repairs > 0 && plural(a.pr_repairs, "PR repair"),
      ].filter(Boolean);
      return { count: a.build, hasRetries: a.build > 1, detail: `${plural(a.build, "pass")}${why.length ? ` (${why.join(", ")})` : ""}` };
    }
    case "test":
      return { count: a.test, hasRetries: a.test_failures > 0, detail: `${plural(a.test, "Windows run")}, ${a.test_failures} failed` };
    case "review":
      return { count: a.review, hasRetries: a.review_fixes > 0, detail: `${plural(a.review, "round")}, ${a.review_fixes} requested fixes` };
    case "pr":
      return { count: a.pr, hasRetries: a.pr_repairs > 0, detail: `opened ${plural(a.pr, "time")}, ${plural(a.pr_repairs, "repair")}` };
    default:
      return { count: 0, hasRetries: false, detail: "" };
  }
}

/**
 * The card's stepper. Attempts come from the board API; when a server does
 * not send them, steps show without counts rather than guessing.
 */
export function progressSteps(ticket: Pick<TicketSummary, "stage" | "work_model" | "attempts">): Step[] {
  const current = currentStepIndex(ticket);
  const attempts = ticket.attempts;
  return PROGRESS_STEPS.map((step, index) => {
    const known = attempts && step.id !== "refine" ? stepDetail(step.id, attempts) : undefined;
    let state: StepState = index < current ? "done" : index === current ? "current" : "upcoming";
    // Passed over with nothing recorded, e.g. a repo without Windows checks.
    if (state === "done" && known && known.count === 0 && (step.id === "test" || step.id === "review")) state = "skipped";
    const status = { done: "done", current: "current step", upcoming: "not started", skipped: "none recorded" }[state];
    const count = known?.count ?? 0;
    return {
      id: step.id,
      label: step.label,
      state,
      count: count > 1 ? count : 0,
      hasRetries: known?.hasRetries ?? false,
      title: `${step.about}: ${status}${known && known.count > 0 ? ` · ${known.detail}` : ""}`,
    };
  });
}
