import type { Repo, TicketDetail, User } from "../../api/types";
import { prTicket } from "../board/boardFixture";

export const deploymentRepo = {id: 2, deploy_mode: "agent", full_name: "example/service", name: "service", provider: "github", url: "https://github.com/example/service"} as Repo;
export const deploymentUser: User = {id: 1, name: "Reader", email: "reader@example.test", is_admin: false, sso_linked: false};
export const deploymentTicket: TicketDetail = {
  ...prTicket, repo_id: 2, lane: "merged", stage: "done", needs_owner: false,
  body: "Improve the application", spec: "", design: "", approved_spec_revision: 0, approved_design_revision: 0,
  gates: {spec: false, design: false}, created_by: "Reader", created_at: "2026-11-01T12:00:00Z",
  questions: [], timeline: [], work_links: [], decisions: [], relations: [],
};
