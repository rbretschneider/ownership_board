import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import type { TicketSummary } from "../../api/types";
import { RunnerNamesContext } from "../../hooks/useRunnerNames";
import { runnerDirectory } from "../../lib/runnerNames";
import { TicketCardContent } from "./TicketCardContent";

const directory = runnerDirectory([{ worker: "codex_1", family: "codex", position: 1, display_name: "Ray", assigned_at: null }]);

const leased: TicketSummary = {
  id: 9, repo_id: null, kind: "story", title: "Ship it", lane: "merged", stage: "deploying",
  owner: "agent", needs_owner: false, blocked_reason: "", is_autonomous: true,
  spec_revision: 1, design_revision: 1, pr_url: "", branch: "feature",
  open_questions: 0, waiting_on: [], lease: { holder: "@worker/7/codex_1", expires_at: "2099-01-01T00:00:00Z" },
  work_model: "gpt-test", created_at: "", updated_at: "", lane_changed_at: "",
};

function card(overrides: Partial<TicketSummary> = {}): HTMLElement {
  const root = document.createElement("div");
  root.innerHTML = renderToStaticMarkup(
    <RunnerNamesContext.Provider value={{ directory, refresh: () => {} }}>
      <TicketCardContent ticket={{ ...leased, ...overrides }} stages={[]} repo={undefined} />
    </RunnerNamesContext.Provider>,
  );
  return root;
}

it("names the lease holder on the card and keeps its identity in the tooltip", () => {
  const chip = card().querySelector<HTMLElement>(".card__lease-holder")!;
  expect(chip.textContent).toBe("Ray (codex_1)");
  expect(chip.getAttribute("title")).toBe("@worker/7/codex_1");
});

it("names the holder in the model chip's tooltip", () => {
  const model = card({ stage: "build", lane: "in_progress" }).querySelector(".card__model");
  expect(model?.getAttribute("title")).toBe("Assigned model for Ray (codex_1)");
});

it("shows a holder outside any pool by its worker id", () => {
  const chip = card({ lease: { holder: "@worker/7/gemini_1", expires_at: "2099-01-01T00:00:00Z" } }).querySelector(".card__lease-holder");
  expect(chip?.textContent).toBe("gemini_1");
});
