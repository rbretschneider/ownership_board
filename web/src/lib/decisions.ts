import type { Decision, DecisionWeight, Question } from "../api/types";

/** Display order of decision weights: the owner's own calls first. */
export const WEIGHT_ORDER: readonly DecisionWeight[] = ["binding", "delegated", "agent"];

/** Badge text and tooltip for each weight. */
export const WEIGHT_LABELS: Readonly<Record<DecisionWeight, { text: string; title: string }>> = {
  binding: { text: "Your decision", title: "A person chose this explicitly; agents must follow it" },
  delegated: { text: "Delegated", title: "You let the AI decide" },
  agent: { text: "AI", title: "Recorded by an agent; context, not binding" },
};

/** Maximum lengths the server accepts for a person's decision. */
export const DECISION_LIMITS = { summary: 300, rationale: 2000, alternatives: 5, alternative: 200 } as const;

/** A ticket's decision log split for display. */
export interface DecisionGroups {
  /** Active decisions, binding → delegated → agent, newest first within each. */
  active: Decision[];
  /** Replaced or removed decisions, newest first. */
  history: Decision[];
}

/** True when a decision is still in force. */
export function isActive(decision: Decision): boolean {
  return !decision.is_removed && decision.superseded_by_id === null;
}

/** Splits decisions into the active list and the collapsed history. */
export function groupDecisions(decisions: readonly Decision[]): DecisionGroups {
  const newestFirst = [...decisions].sort(byNewest);
  const active = newestFirst.filter(isActive).sort((a, b) => weightRank(a) - weightRank(b));
  return { active, history: newestFirst.filter((d) => !isActive(d)) };
}

/** The summary of the decision that replaced `decision`, if it is known. */
export function replacementSummary(decision: Decision, all: readonly Decision[]): string | null {
  if (decision.superseded_by_id === null) {
    return null;
  }
  const replacement = all.find((d) => d.id === decision.superseded_by_id);
  if (replacement === undefined || replacement.is_removed) {
    return null;
  }
  return replacement.summary;
}

/** The question round an answer-derived decision came from, if known. */
export function decisionRound(decision: Decision, questions: readonly Question[]): number | null {
  if (decision.question_id === null) {
    return null;
  }
  return questions.find((q) => q.id === decision.question_id)?.round ?? null;
}

/** Explains what deleting a decision does, for the confirm dialog. */
export function deleteMessage(decision: Decision): string {
  const base = "It stays in the log as a “Removed” line without its text.";
  return decision.supersedes_id === null ? base : `${base} The decision it replaced becomes active again.`;
}

/** Trims a person's alternatives: drops blanks and keeps at most the limit. */
export function cleanAlternatives(alternatives: readonly string[]): string[] {
  return alternatives.map((a) => a.trim()).filter((a) => a !== "").slice(0, DECISION_LIMITS.alternatives);
}

function weightRank(decision: Decision): number {
  return WEIGHT_ORDER.indexOf(decision.weight);
}

function byNewest(a: Decision, b: Decision): number {
  return b.at.localeCompare(a.at) || b.id - a.id;
}
