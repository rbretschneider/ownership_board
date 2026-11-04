import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { ReactElement } from "react";
import type { RunnersSnapshot, TicketSummary } from "../../api/types";
import { RunnersContext } from "../../hooks/useRunners";
import { RunnersStrip } from "./RunnersStrip";
import { TicketCardContent } from "./TicketCardContent";

const NOW = Date.parse("2026-11-04T20:30:00Z");
const snapshot: RunnersSnapshot = {
 observed_at: "2026-11-04T20:29:58Z", host: "Ryan-Desktop", enabled: true, parallel_builds: 3,
 slots: [
  { slot: 1, ticket_id: 24, repo: "acme/cad", sha: "8c19d1c048ff", preset: "script", phase: "verifying", detail: "ASan · testing", since: "2026-11-04T20:00:51Z", updated_at: null, orphan: false },
  { slot: 3, ticket_id: 85, repo: "acme/cad", sha: "958941de5c2b", preset: "script", phase: "verifying", detail: "waiting for desktop", since: "2026-11-04T20:25:00Z", updated_at: null, orphan: false },
 ],
 queue: [
  { position: 1, ticket_id: 27, repo: "acme/cad", sha: "56f13b70b6ec", preset: "cmake", queued_at: null, deferred_until: null },
  { position: 2, ticket_id: null, repo: "acme/mix", sha: "00d05479aaaa", preset: "dotnet", queued_at: null, deferred_until: null },
 ],
};
function dom(element: ReactElement, value: RunnersSnapshot = snapshot, now = NOW) {
 const root = document.createElement("div");
 root.innerHTML = renderToStaticMarkup(<RunnersContext.Provider value={{ snapshot: value, now }}>{element}</RunnersContext.Provider>);
 return root;
}

describe("runners strip", () => {
 it("shows every slot with its ticket, step and time, idle slots, and who is next", () => {
  const strip = dom(<RunnersStrip />);
  const chips = [...strip.querySelectorAll(".runner")];
  expect(chips.map((c) => c.textContent)).toEqual(["1#24ASan · testing29m", "2idle", "3#85waiting for desktop5m"]);
  expect(chips[0]?.className).toContain("runner--testing");
  expect(chips[2]?.className).toContain("runner--waiting");
  expect(chips[0]?.getAttribute("href")).toBe("#/t/24");
  expect(strip.querySelector(".runners__queue")?.textContent).toBe("Next:#2700d0547");
 });
 it("flags an orphaned run and a host that stopped reporting", () => {
  const orphaned = { ...snapshot, slots: [{ ...snapshot.slots[0]!, orphan: true }] };
  expect(dom(<RunnersStrip />, orphaned).querySelector(".runner--orphan")?.textContent).toContain("old commit · stopping");
  const offline = dom(<RunnersStrip />, snapshot, NOW + 10 * 60_000);
  expect(offline.querySelector(".runners")?.className).toContain("runners--offline");
  expect(offline.querySelector(".runners__state")?.textContent).toBe("not reporting · 10m");
  expect(dom(<RunnersStrip />, { ...snapshot, observed_at: null }).innerHTML).toBe("");
 });
});

describe("card pill follows the build host", () => {
 const ticket: TicketSummary = {
  id: 24, repo_id: null, kind: "story", title: "Composition root", lane: "in_progress", stage: "build", owner: "agent", needs_owner: false,
  blocked_reason: "", is_autonomous: true, spec_revision: 1, design_revision: 1, pr_url: "", branch: "b", open_questions: 0, waiting_on: [],
  lease: { holder: "codex_3", expires_at: "2099-01-01T00:00:00Z" }, work_model: "gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows",
  created_at: "", updated_at: "", lane_changed_at: "",
 };
 const pill = (t: TicketSummary, value = snapshot) => dom(<TicketCardContent ticket={t} stages={[]} repo={undefined} now={NOW} />, value).querySelector(".card__activity");
 it("shows the pass and step of a running job, even when the worker last said queued", () => {
  expect(pill(ticket)?.querySelector(".card__activity-label")?.textContent).toBe("ASan · testing");
  expect(pill(ticket)?.className).toContain("badge--build-server");
  expect(pill(ticket)?.getAttribute("title")).toContain("slot 1");
 });
 it("shows the place in line and an old run being stopped", () => {
  expect(pill({ ...ticket, id: 27 })?.querySelector(".card__activity-label")?.textContent).toBe("Queued #1");
  const orphaned = { ...snapshot, slots: [{ ...snapshot.slots[0]!, orphan: true }], queue: [] };
  expect(pill(ticket, orphaned)?.querySelector(".card__activity-label")?.textContent).toBe("Old run stopping");
 });
 it("keeps the worker's signal when the host has not reported", () => {
  expect(pill(ticket, { ...snapshot, observed_at: null })?.querySelector(".card__activity-label")?.textContent).toBe("Queued");
 });
});
