import type { Lane, TicketSummary } from "../../api/types";

export const lanes: Lane[] = [
  { id: "todo", label: "Todo", description: "Ideas" },
  { id: "in_progress", label: "In progress", description: "Active work" },
  { id: "merged", label: "Merged", description: "Integrated work" },
  { id: "deployed", label: "Deployed", description: "Released work" },
];
export const prTicket: TicketSummary = {
  id: 1, repo_id: null, kind: "story", title: "Improve navigation", lane: "in_progress", stage: "pr",
  owner: "human", needs_owner: true, blocked_reason: "", is_autonomous: false,
  spec_revision: 1, design_revision: 1, pr_url: "https://github.com/acme/app/pull/1", branch: "feature",
  open_questions: 0, waiting_on: [], lease: null, created_at: "", updated_at: "", lane_changed_at: "",
};
