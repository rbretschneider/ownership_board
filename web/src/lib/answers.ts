import type { Answer, Question, TimelineEntry } from "../api/types";

/** The human's in-progress answer to one question. */
export interface AnswerDraft {
  choice: number | null;
  text: string;
  deferred: boolean;
}

/** Drafts keyed by question id. */
export type DraftMap = Readonly<Record<number, AnswerDraft>>;

/** A draft with nothing chosen yet. */
export const EMPTY_DRAFT: AnswerDraft = { choice: null, text: "", deferred: false };

/** Questions the human has not answered yet, in asking order. */
export function openQuestions(questions: readonly Question[]): Question[] {
  return questions.filter((q) => q.answer === null);
}

/** Answered questions grouped by round, oldest round first. */
export function answeredRounds(questions: readonly Question[]): { round: number; questions: Question[] }[] {
  const rounds = new Map<number, Question[]>();
  for (const question of questions) {
    if (question.answer !== null) {
      rounds.set(question.round, [...(rounds.get(question.round) ?? []), question]);
    }
  }
  return [...rounds.entries()].sort(([a], [b]) => a - b).map(([round, qs]) => ({ round, questions: qs }));
}

/** True when the draft says something: a choice, some text, or "your call". */
export function isDraftComplete(draft: AnswerDraft | undefined): boolean {
  if (draft === undefined) {
    return false;
  }
  return draft.choice !== null || draft.text.trim() !== "" || draft.deferred;
}

/** True when every open question has a complete draft. */
export function areAllAnswered(open: readonly Question[], drafts: DraftMap): boolean {
  return open.length > 0 && open.every((q) => isDraftComplete(drafts[q.id]));
}

/**
 * Answers every open question with the agent's recommendation, or "your call"
 * where no option was recommended. Typed text is kept.
 */
export function acceptRecommendations(open: readonly Question[], drafts: DraftMap): DraftMap {
  const next: Record<number, AnswerDraft> = { ...drafts };
  for (const question of open) {
    const current = drafts[question.id] ?? EMPTY_DRAFT;
    next[question.id] = hasRecommendation(question)
      ? { ...current, choice: question.recommended, deferred: false }
      : { ...current, choice: null, deferred: true };
  }
  return next;
}

/** Converts drafts into the request body for POST /tickets/{id}/answers. */
export function toAnswers(open: readonly Question[], drafts: DraftMap): Answer[] {
  return open.map((question) => {
    const draft = drafts[question.id] ?? EMPTY_DRAFT;
    return {
      question_id: question.id,
      choice: draft.choice,
      text: draft.text.trim(),
      deferred: draft.deferred,
    };
  });
}

/** The agent's summary for a question round, taken from the timeline. */
export function roundSummary(timeline: readonly TimelineEntry[], round: number): string {
  const entry = [...timeline].reverse().find((e) => e.type === "questions" && e.data["round"] === round);
  return entry?.body ?? "";
}

function hasRecommendation(question: Question): question is Question & { recommended: number } {
  return question.recommended !== null && question.recommended >= 0 && question.recommended < question.options.length;
}
