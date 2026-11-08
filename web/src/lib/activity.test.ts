import { describe, expect, it } from "vitest";
import type { TicketSummary } from "../api/types";
import { cardActivity, cardTone } from "./activity";
const ticket = { stage: "build", lane: "in_progress", blocked_reason: "", waiting_on: [], needs_owner: false, open_questions: 0, lease: {holder: "codex_1", expires_at: "2099-01-01"}, work_model: "gpt-6.1-sol" } as unknown as TicketSummary;
describe("card activity", () => {
  it("separates queued and active target builds from agent work", () => {
    const offload = (phase: string) => ({...ticket, work_model: `gpt-6.1-sol · build-server:${phase}:Windows x64 · awaiting Windows`});
    expect(cardActivity(offload("queued"))).toMatchObject({label: "Queued · Windows x64", working: false, activeBuild: false});
    expect(cardTone(offload("queued"))).toBe(" card--idle");
    expect(cardActivity(offload("building"))).toMatchObject({label: "Building · Windows x64", working: false, activeBuild: true});
    expect(cardTone(offload("building"))).toBe(" card--build-server");
    expect(cardActivity(offload("testing")).label).toBe("Testing · Windows x64");
    expect(cardActivity({...offload("building"), lease: null}).activeBuild).toBe(false);
    expect(cardActivity({...offload("paused"), lease: null}).label).toBe("Paused · Windows x64");
    expect(cardActivity({...offload("paused"), lease: {holder: "old", expires_at: "2000-01-01"}}).activeBuild).toBe(false);
    expect(cardTone({...offload("building"), blocked_reason: "Missing access"})).toBe(" card--blocked");
  });
  it("shows actual work and explicit fresh review activity", () => {
    expect(cardActivity({...ticket, work_model:"gpt-6.1-sol preparing workspace"})).toMatchObject({label:"Preparing",working:true});
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

it("labels completed work Merged", () => {
 expect(cardActivity({...ticket, stage: "done"}).label).toBe("Merged");
});

it("keeps deployed work completed even if old worker metadata remains", () => {
 expect(cardActivity({...ticket, lane: "deployed", stage: "deployed"})).toEqual({label: "Deployed", working: false, blocked: false});
 expect(cardTone({...ticket, lane: "deployed", stage: "deployed"})).toBe("");
});
