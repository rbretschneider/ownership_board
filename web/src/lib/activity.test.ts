import { describe, expect, it } from "vitest";
import type { TicketSummary } from "../api/types";
import { cardActivity, cardTone } from "./activity";
const ticket = { stage: "build", lane: "in_progress", blocked_reason: "", waiting_on: [], needs_owner: false, open_questions: 0, lease: {holder: "codex_1", expires_at: "2099-01-01"}, work_model: "gpt-6.1-sol" } as unknown as TicketSummary;
describe("card activity", () => {
  it("shows actual work and explicit fresh review activity", () => {
    expect(cardActivity(ticket)).toMatchObject({label: "Building", working: true});
    expect(cardActivity({...ticket, stage: "refine"}).label).toBe("Refining");
    expect(cardActivity({...ticket, work_model: "gpt-6.1-sol � adversarial review"}).label).toBe("Reviewing");
    expect(cardActivity({...ticket, work_model: "gpt-6.1-sol � awaiting review"})).toMatchObject({label: "Awaiting review", working: false});
  });
  it("gives blockers priority over an active claim", () => {
    expect(cardTone({...ticket, blocked_reason: "Access needed"})).toBe(" card--blocked");
    expect(cardTone({...ticket, waiting_on: [12]})).toBe(" card--blocked");
    expect(cardTone(ticket)).toBe(" card--working");
  });
  it("never animates queued work or an inactive reviewer model", () => {
    expect(cardActivity({...ticket, lease: null, work_model: "gpt-6.1-sol � adversarial review"})).toMatchObject({label: "Queued", working: false});
    expect(cardActivity({...ticket, stage: "pr", lease: null})).toMatchObject({label: "Awaiting review", working: false});
    expect(cardTone({...ticket, stage: "ready", lane: "todo", lease: null})).toBe("");
    expect(cardTone({...ticket, lease: null})).toBe(" card--idle");
    expect(cardTone({...ticket, stage: "done"})).toBe("");
  });
});
