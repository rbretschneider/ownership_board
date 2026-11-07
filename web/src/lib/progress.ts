import type { StageAttempts, TicketSummary } from "../api/types";

export type StepId = "refine" | "build" | "review" | "pr";

export const PROGRESS_STEPS: { id: StepId; label: string; about: string }[] = [
  { id: "refine", label: "Refine", about: "Refine and design" },
  { id: "build", label: "Build", about: "Build and test" },
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

function buildSignal(ticket: Pick<TicketSummary, "work_model">): string {
  return ticket.work_model?.split(" · ").slice(1).join(" · ") ?? "";
}

/**
 * The ticket is in build, waiting on or running Windows verification. That is
 * part of Build (every repo runs its tests there), not a step of its own: the
 * card shows it as a status.
 */
export function onWindows(ticket: Pick<TicketSummary, "stage" | "work_model">): boolean {
  const signal = buildSignal(ticket);
  return ticket.stage === "build" && (signal.startsWith("build-server:") || signal === "awaiting Windows" || signal === "awaiting verification");
}

/**
 * Which step the ticket is on (0–3), or 4 once delivered. Within build, the
 * work-model signal says whether the code is being built (written, tested,
 * verified on Windows) or reviewed.
 */
export function currentStepIndex(ticket: Pick<TicketSummary, "stage" | "work_model">): number {
  switch (ticket.stage) {
    case "build": {
      const signal = buildSignal(ticket);
      if (!onWindows(ticket) && (signal.includes("adversarial review") || signal === "awaiting review")) return 2;
      return 1;
    }
    case "pr":
      return 3;
    case "done":
      return 4;
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
    // Passed over with nothing recorded.
    if (state === "done" && known && known.count === 0 && step.id === "review") state = "skipped";
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

export interface WindowsStatus {
  passed: boolean;
  label: string;
  title: string;
}

/**
 * Windows verification as a simple status on the card, or undefined when none
 * ran (most repos have no Windows check) or while it is under way (the status
 * pill shows that live). A passing run ends the Windows loop, so more runs than
 * failures means the latest one passed.
 */
export function windowsStatus(ticket: Pick<TicketSummary, "stage" | "work_model" | "attempts">): WindowsStatus | undefined {
  const a = ticket.attempts;
  if (a === undefined || a.test === 0 || onWindows(ticket)) return undefined;
  const passed = a.test > a.test_failures;
  const history = `${plural(a.test, "Windows run")}, ${a.test_failures} failed`;
  return {
    passed,
    label: passed ? "Windows passed" : "Windows failed",
    title: `Windows verification ${passed ? "passed" : "failed"} · ${history}`,
  };
}
