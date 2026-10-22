import {
  CircleDashed,
  ClipboardCheck,
  DraftingCompass,
  FileCheck,
  GitMerge,
  GitPullRequest,
  Hammer,
  PenLine,
} from "lucide-react";
import type { Stage, StageId } from "../../api/types";
import { stageLabel } from "../../lib/stages";

const ICONS: Record<StageId, typeof PenLine> = {
  refine: PenLine,
  spec_review: FileCheck,
  ready: CircleDashed,
  design: DraftingCompass,
  design_review: ClipboardCheck,
  build: Hammer,
  pr: GitPullRequest,
  done: GitMerge,
};

/** A ticket's stage as icon plus text. */
export function StageChip({ stage, stages }: { stage: StageId; stages: Stage[] }) {
  const Icon = ICONS[stage] ?? CircleDashed;
  return (
    <span className="stage-chip">
      <Icon size={13} aria-hidden="true" />
      {stageLabel(stages, stage)}
    </span>
  );
}
