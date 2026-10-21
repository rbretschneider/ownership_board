import { describe, expect, it } from "vitest";
import type { Question, TimelineEntry } from "../api/types";
import {
  acceptRecommendations,
  answeredRounds,
  areAllAnswered,
  EMPTY_DRAFT,
  isDraftComplete,
  openQuestions,
  ownDraft,
  roundSummary,
  toAnswers,
} from "./answers";

function question(id: number, overrides: Partial<Question> = {}): Question {
  return {
    id, round: 1, prompt: `q${id}`, options: ["a", "b"], recommended: null,
    asked_by: "agent", asked_at: "", answer: null, ...overrides,
  };
}

const answered = { choice: 0, text: "", deferred: false, by: "ryan", at: "" };

describe("isDraftComplete", () => {
  it("is false for an empty draft", () => {
    expect(isDraftComplete(EMPTY_DRAFT)).toBe(false);
  });
  it("is false for whitespace-only text", () => {
    expect(isDraftComplete({ ...EMPTY_DRAFT, text: "   " })).toBe(false);
  });
  it("accepts a choice, text, or your call", () => {
    expect(isDraftComplete({ ...EMPTY_DRAFT, choice: 0 })).toBe(true);
    expect(isDraftComplete({ ...EMPTY_DRAFT, text: "yes" })).toBe(true);
    expect(isDraftComplete({ ...EMPTY_DRAFT, deferred: true })).toBe(true);
  });
  it("is false for a missing draft", () => {
    expect(isDraftComplete(undefined)).toBe(false);
  });
});

describe("areAllAnswered", () => {
  const open = [question(1), question(2)];
  it("requires every open question", () => {
    expect(areAllAnswered(open, { 1: { ...EMPTY_DRAFT, choice: 1 } })).toBe(false);
  });
  it("passes when all are complete", () => {
    expect(areAllAnswered(open, { 1: { ...EMPTY_DRAFT, choice: 1 }, 2: { ...EMPTY_DRAFT, deferred: true } })).toBe(true);
  });
  it("is false with nothing open", () => {
    expect(areAllAnswered([], {})).toBe(false);
  });
});

describe("acceptRecommendations", () => {
  it("picks the recommendation, or your call when there is none", () => {
    const open = [question(1, { recommended: 1 }), question(2)];
    const drafts = acceptRecommendations(open, {});
    expect(drafts[1]).toEqual({ choice: 1, text: "", deferred: false, isBulk: true });
    expect(drafts[2]).toEqual({ choice: null, text: "", deferred: true, isBulk: true });
  });
  it("leaves answers the person already gave as their own", () => {
    const open = [question(1, { recommended: 1 }), question(2, { recommended: 0 })];
    const mine = { ...EMPTY_DRAFT, choice: 1, text: "keep me" };
    const drafts = acceptRecommendations(open, { 2: mine });
    expect(drafts[2]).toEqual(mine);
    expect(drafts[1]?.isBulk).toBe(true);
  });
  it("treats an out-of-range recommendation as none", () => {
    const drafts = acceptRecommendations([question(1, { recommended: 5 })], {});
    expect(drafts[1]?.deferred).toBe(true);
  });
});

describe("bulk tracking", () => {
  const open = [question(1, { recommended: 0 }), question(2, { recommended: 1 })];
  it("sends bulk for answers filled by Accept recommendations", () => {
    const answers = toAnswers(open, acceptRecommendations(open, {}));
    expect(answers.map((a) => a.bulk)).toEqual([true, true]);
  });
  it("makes an answer the person's own once they edit it after the bulk fill", () => {
    const filled = acceptRecommendations(open, {});
    const edited = { ...filled, 2: ownDraft({ ...EMPTY_DRAFT, ...filled[2], choice: 0 }) };
    expect(toAnswers(open, edited).map((a) => a.bulk)).toEqual([true, false]);
  });
  it("never marks a hand-picked answer as bulk", () => {
    const answers = toAnswers([question(1)], { 1: ownDraft({ ...EMPTY_DRAFT, choice: 0 }) });
    expect(answers[0]?.bulk).toBe(false);
  });
});

describe("toAnswers", () => {
  it("builds the request body with trimmed text", () => {
    const answers = toAnswers([question(1)], { 1: { choice: 0, text: " note ", deferred: false, isBulk: false } });
    expect(answers).toEqual([{ question_id: 1, choice: 0, text: "note", deferred: false, bulk: false }]);
  });
});

describe("openQuestions and answeredRounds", () => {
  const questions = [
    question(1, { round: 1, answer: answered }),
    question(2, { round: 2, answer: answered }),
    question(3, { round: 2 }),
  ];
  it("lists only unanswered questions", () => {
    expect(openQuestions(questions).map((q) => q.id)).toEqual([3]);
  });
  it("groups answered questions by round", () => {
    expect(answeredRounds(questions).map((r) => [r.round, r.questions.map((q) => q.id)])).toEqual([[1, [1]], [2, [2]]]);
  });
});

describe("roundSummary", () => {
  const entry = (id: number, round: number, body: string): TimelineEntry => ({
    id, type: "questions", at: "", actor: { kind: "agent", name: "claude" }, body, data: { round },
  });
  it("finds the summary for the round", () => {
    expect(roundSummary([entry(1, 1, "first"), entry(2, 2, "second")], 2)).toBe("second");
  });
  it("is empty when missing", () => {
    expect(roundSummary([], 1)).toBe("");
  });
});
