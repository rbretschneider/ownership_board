import type { StageId } from "../api/types";

export const PROGRESS_STEPS = ["Refinement", "Engineering plan", "Build and tests", "Review and checks", "Delivery"];

// Stage-based milestones, not a time estimate. Checkpoints leave their phase
// incomplete until approved; returning to an earlier stage lowers the count.
export function completedSteps(stage: StageId): number {
  const completed: Record<StageId, number> = {
    refine: 0, spec_review: 0, ready: 1, design: 1,
    design_review: 1, build: 2, pr: 3, done: 5,
  };
  return completed[stage];
}
