import { Bug, BookOpen } from "lucide-react";
import type { Kind } from "../../api/types";

const KIND_LABELS: Record<Kind, string> = { story: "Story", bug: "Bug" };

/** Story/Bug tag with icon and text. */
export function KindTag({ kind }: { kind: Kind }) {
  const Icon = kind === "bug" ? Bug : BookOpen;
  return (
    <span className={`kind-tag kind-tag--${kind}`}>
      <Icon size={13} aria-hidden="true" />
      {KIND_LABELS[kind]}
    </span>
  );
}
