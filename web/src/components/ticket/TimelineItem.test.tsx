import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it } from "vitest";
import type { TimelineEntry } from "../../api/types";
import { RunnerNamesContext } from "../../hooks/useRunnerNames";
import { runnerDirectory } from "../../lib/runnerNames";
import { TimelineItem } from "./TimelineItem";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

async function renderEntry(entry: TimelineEntry): Promise<string> {
  const host = document.createElement("ul");
  const root = createRoot(host);
  await act(async () => { root.render(<TimelineItem entry={entry} lanes={[]} stages={[]} />); });
  const text = host.textContent ?? "";
  await act(async () => { root.unmount(); });
  return text;
}

function ciEntry(data: Record<string, unknown>): TimelineEntry {
  return { id: 1, type: "ci", at: "2026-11-16T10:00:00Z", actor: { kind: "agent", name: "board-manager" }, body: "evicted by a concurrency group", data };
}

it("says which CI run the manager cancelled, with its reason", async () => {
  const text = await renderEntry(ciEntry({ action: "cancel_run", run_id: 42, workflow: "CI" }));
  expect(text).toContain("cancelled CI run 42 (CI)");
  expect(text).toContain("evicted by a concurrency group");
});

it("names a runner's comments, with its raw identity in the tooltip", async () => {
  const directory = runnerDirectory([{ worker: "claude_1", family: "claude", position: 1, display_name: "Michael", assigned_at: null }]);
  const host = document.createElement("ul");
  const root = createRoot(host);
  const comment: TimelineEntry = { id: 2, type: "comment", at: "2026-11-16T10:00:00Z", actor: { kind: "agent", name: "@worker/7/claude_1" }, body: "Done", data: {} };
  await act(async () => {
    root.render(<RunnerNamesContext.Provider value={{ directory, refresh: () => {} }}><TimelineItem entry={comment} lanes={[]} stages={[]} /></RunnerNamesContext.Provider>);
  });
  const author = host.querySelector<HTMLElement>(".comment__author .runner-name")!;
  expect(author.textContent).toBe("Michael (claude_1)");
  expect(author.title).toBe("@worker/7/claude_1");
  await act(async () => { root.unmount(); });
});

it("says which job of a CI run was re-run", async () => {
  const text = await renderEntry(ciEntry({ action: "rerun_job", run_id: 42, job_id: 7, workflow: "" }));
  expect(text).toContain("re-ran job 7 of CI run 42");
});
