import { describe, expect, it } from "vitest";
import type { RunnerName } from "../api/types";
import { EMPTY_RUNNER_DIRECTORY, labelRunner, nameRunnersInText, runnerDirectory } from "./runnerNames";

const named = (worker: string, family: string, displayName: string): RunnerName =>
  ({ worker, family, position: 1, display_name: displayName, assigned_at: null });

const directory = runnerDirectory([named("claude_1", "claude", "Michael"), named("codex_1", "codex", "Ray")]);

describe("labelRunner", () => {
  it("names a pooled runner and keeps its identity for tooltips", () => {
    expect(labelRunner("@worker/7/claude_1", directory)).toEqual({
      label: "Michael", detail: "Michael (claude_1)", title: "@worker/7/claude_1", badge: "",
    });
  });

  it("falls back to the worker id, or the identity for a plain agent token", () => {
    expect(labelRunner("@worker/7/gemini_1", directory).label).toBe("gemini_1");
    expect(labelRunner("claude-loop", directory)).toEqual({ label: "claude-loop", detail: "claude-loop", title: "claude-loop", badge: "" });
  });

  it("uses a name the payload carries when the directory lacks it", () => {
    expect(labelRunner("@worker/7/claude_3", EMPTY_RUNNER_DIRECTORY, { display_name: "Jim", family: "claude" }).detail).toBe("Jim (claude_3)");
  });

  it("badges the family only when another family shares the name", () => {
    const clash = runnerDirectory([named("claude_1", "claude", "Michael"), named("codex_9", "codex", "michael")]);
    expect(labelRunner("@worker/7/claude_1", clash).badge).toBe("claude");
    expect(labelRunner("@worker/7/codex_9", clash).badge).toBe("codex");
    expect(labelRunner("@worker/7/claude_1", directory).badge).toBe("");
  });
});

describe("nameRunnersInText", () => {
  it("names runners a manager summary mentions, by identity or bare worker id", () => {
    expect(nameRunnersInText("Moved @worker/7/claude_1 off #12; codex_1 took it.", directory))
      .toBe("Moved Michael (claude_1) off #12; Ray (codex_1) took it.");
  });

  it("leaves unknown workers and already named mentions alone", () => {
    expect(nameRunnersInText("gemini_1 idle; Michael (claude_1) busy", directory)).toBe("gemini_1 idle; Michael (claude_1) busy");
  });
});
