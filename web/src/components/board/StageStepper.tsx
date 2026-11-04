import type { TicketSummary } from "../../api/types";
import { progressSteps } from "../../lib/progress";

/**
 * Refine → Build → Test → Review → PR as five segments (done filled, current
 * highlighted), with one line under them naming the current step and every
 * step that ran more than once, e.g. "Build 4 → Test 5". Hover or focus a
 * segment for its full history.
 */
export function StageStepper({ ticket }: { ticket: TicketSummary }) {
  const steps = progressSteps(ticket);
  const named = steps.filter((step) => step.state === "current" || step.count > 0);
  return (
    <div className="stepper">
      <ol className="stepper__track" aria-label="Workflow progress">
        {steps.map((step) => (
          <li
            key={step.id}
            className={`stepper__step stepper__step--${step.state}`}
            title={step.title}
            aria-label={step.title}
            aria-current={step.state === "current" ? "step" : undefined}
          />
        ))}
      </ol>
      {named.length > 0 && (
        <p className="stepper__summary" aria-hidden="true">
          {named.map((step) => (
            <span key={step.id} className={`stepper__item${step.state === "current" ? " stepper__item--current" : ""}${step.hasRetries ? " stepper__item--retried" : ""}`}>
              <span className="stepper__label">{step.label}</span>
              {step.count > 0 && <span className="stepper__count">{step.count}</span>}
            </span>
          ))}
        </p>
      )}
    </div>
  );
}
