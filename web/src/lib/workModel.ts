/**
 * The model part of a ticket's work_model signal ("<model> · <signal> · ...").
 * A worker that moved a repeatedly rejected story onto a stronger model tags
 * it as "<model> [escalated:<rejections>]".
 */
export interface WorkModel {
  name: string;
  /** Rejected code passes that triggered the stronger model; absent when not escalated. */
  escalatedAfter?: number;
}

export function parseWorkModel(workModel: string | undefined): WorkModel | undefined {
  const head = workModel?.split(" · ")[0]?.trim();
  if (!head) return undefined;
  const tagged = head.match(/^(.+?)\s*\[escalated:(\d+)\]$/);
  if (tagged && tagged[1] && tagged[2]) return { name: tagged[1], escalatedAfter: Number(tagged[2]) };
  return { name: head };
}

export type ModelFamily = "claude" | "codex";

/** Which model family a reported model name belongs to, or undefined when unknown. */
export function modelFamily(name: string | undefined): ModelFamily | undefined {
  if (!name) return undefined;
  if (/\b(claude|opus|sonnet|haiku|fable)\b/i.test(name)) return "claude";
  if (/\b(codex|gpt)\b|^gpt-|^o\d/i.test(name)) return "codex";
  return undefined;
}

/** "Windows x64" reads as "Win x64" inside a compact status pill. */
export function shortTarget(target: string): string {
  return target.replace(/^Windows\b/, "Win");
}
