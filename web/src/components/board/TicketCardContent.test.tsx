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
