import type { TicketSummary } from "../../api/types";
import { progressSteps } from "../../lib/progress";

/**
 * Refine › Build › Test › Review › PR as chevrons along the card's bottom
 * edge. Done steps are filled, the current one is solid accent, and a step
 * that ran more than once carries its count inside (e.g. Build 4 › Test 5).
 * Hover or focus a step for its full history.
 */
export function StageStepper({ ticket }: { ticket: TicketSummary }) {
  const steps = progressSteps(ticket);
  return (
    <ol className="stepper" aria-label="Workflow progress">
      {steps.map((step) => (
        <li
          key={step.id}
          className={`stepper__step stepper__step--${step.state}${step.count > 0 ? " stepper__step--counted" : ""}${step.hasRetries ? " stepper__step--retried" : ""}`}
          title={step.title}
          aria-label={step.title}
          aria-current={step.state === "current" ? "step" : undefined}
        >
          <span className="stepper__label" aria-hidden="true">{step.label}</span>
          {step.count > 0 && <span className="stepper__count" aria-hidden="true">{step.count}</span>}
        </li>
      ))}
    </ol>
  );
}
