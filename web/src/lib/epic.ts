import type { SplitChild, TicketSummary } from "../api/types";
import { isCompletedStage } from "./stages";

/** Where one story of an epic stands, coarsely, for the progress bar. */
export type EpicPartState = "deployed" | "merged" | "active" | "waiting";

/** An epic's progress over the stories it was split into. */
export interface EpicProgress {
  total: number;
  deployed: number;
  /** Merged but not deployed yet. */
  merged: number;
  parts: { child: SplitChild; state: EpicPartState }[];
  /** e.g. "2 of 5 deployed". */
  label: string;
}

/** A story's coarse state: deployed, merged, in progress, or not started. */
export function epicPartState(child: Pick<SplitChild, "stage" | "lane">): EpicPartState {
  if (child.stage === "deployed") return "deployed";
  if (isCompletedStage(child.stage)) return "merged";
  return child.lane === "todo" ? "waiting" : "active";
}

/** The progress of a ticket that was split, or null when it was not. */
export function epicProgress(ticket: Pick<TicketSummary, "split_into">): EpicProgress | null {
  const children = ticket.split_into ?? [];
  if (children.length === 0) return null;
  const parts = children.map((child) => ({ child, state: epicPartState(child) }));
  const deployed = parts.filter((p) => p.state === "deployed").length;
  const merged = parts.filter((p) => p.state === "merged").length;
  return { total: children.length, deployed, merged, parts, label: `${deployed} of ${children.length} deployed` };
}
