import { ApiError } from "../api/ApiError";
import type { Checkpoint, LaneId, Stage, StageId, TicketDetail, TicketSummary } from "../api/types";

/** One entry of a column's stage summary. */
export interface StageCount {
  id: StageId;
  label: string;
  count: number;
}

/**
 * Per-stage ticket counts for one lane, in workflow order. Stages with no
 * tickets are left out so the summary stays one compact line.
 */
export function stageCounts(stages: Stage[], tickets: TicketSummary[], lane: LaneId): StageCount[] {
  return stages
    .filter((s) => s.lane === lane)
    .map((s) => ({ id: s.id, label: s.label, count: tickets.filter((t) => t.stage === s.id).length }))
    .filter((s) => s.count > 0);
}

/** The summary line under a column header, e.g. "Refining 3 · Spec ready 1". */
export function stageSummary(counts: StageCount[]): string {
  return counts.map((c) => `${c.label} ${c.count}`).join(" · ");
}

/** The stage's display name, falling back to its id for a stage the UI does not know. */
export function stageLabel(stages: Stage[], id: StageId): string {
  return stages.find((s) => s.id === id)?.label ?? id;
}

/** Tickets waiting on the owner: the "Your move" filter. */
export function needsOwner(tickets: TicketSummary[]): TicketSummary[] {
  return tickets.filter((t) => t.needs_owner);
}

/** The checkpoint a stage waits on, or null when the stage is not a checkpoint. */
export function checkpointFor(stage: StageId): Checkpoint | null {
  switch (stage) {
    case "spec_review":
      return "spec";
    case "design_review":
      return "design";
    default:
      return null;
  }
}

/** The text and revision the owner is approving at a checkpoint. */
export function checkpointDocument(ticket: TicketDetail, checkpoint: Checkpoint): { text: string; revision: number } {
  return checkpoint === "spec"
    ? { text: ticket.spec, revision: ticket.spec_revision }
    : { text: ticket.design, revision: ticket.design_revision };
}

/** Whether the shown revision is the approved one (revision 0 is never approved). */
export function isApprovedRevision(approved: number, current: number): boolean {
  return current > 0 && approved === current;
}

/** Validation message for a request-changes comment, or null when it may be sent. */
export function requestChangesError(comment: string): string | null {
  return comment.trim() === "" ? "Say what should change." : null;
}

/** Shown when a checkpoint changed between opening the sheet and acting on it. */
export const STALE_CHECKPOINT_MESSAGE = "This changed since you opened it — reload to see the latest";

/** Whether a failed checkpoint call means the ticket moved on (409), so the owner should reload. */
export function isStaleConflict(cause: unknown): boolean {
  return cause instanceof ApiError && cause.status === 409;
}
