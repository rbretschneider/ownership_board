import { expect, it } from "vitest";
import type { Repo, User } from "../api/types";
import { prTicket } from "../components/board/boardFixture";
import { canRequestDeploy } from "./deployment";

const ticket = {...prTicket, repo_id: 2, lane: "merged" as const, stage: "done" as const};
const repo = {id: 2, deploy_mode: "agent"} as Repo;
const user: User = {id: 1, name: "Reader", email: "reader@example.test", is_admin: false, sso_linked: false};
it("permits authenticated humans including non-administrators", () => {
  expect(canRequestDeploy(ticket, repo, user)).toBe(true);
  expect(canRequestDeploy(ticket, repo, {...user, is_admin: true})).toBe(true);
});
it("requires a human and matching deployment-enabled repository", () => {
  expect(canRequestDeploy(ticket, repo, null)).toBe(false);
  for (const candidate of [undefined, {...repo, id: 3}, {...repo, deploy_mode: "off"}, {id: 2}] as (Repo | undefined)[]) {
    expect(canRequestDeploy(ticket, candidate, user)).toBe(false);
  }
  expect(canRequestDeploy({...ticket, repo_id: null}, repo, user)).toBe(false);
});
it("requires an unblocked merged ticket awaiting deployment", () => {
  for (const stage of ["refine", "ready", "design", "build", "pr", "deploying", "deployed"] as const) {
    expect(canRequestDeploy({...ticket, stage}, repo, user)).toBe(false);
  }
  for (const lane of ["todo", "in_progress", "deployed"] as const) {
    expect(canRequestDeploy({...ticket, lane}, repo, user)).toBe(false);
  }
  const blocked = {...ticket, blocked_reason: "Deployment failed"};
  expect(canRequestDeploy(blocked, repo, user)).toBe(false);
  expect(canRequestDeploy({...blocked, blocked_reason: ""}, repo, user)).toBe(true);
});
