import { useRunnerLabel } from "../../hooks/useRunnerNames";

/** Props for {@link RunnerNameTag}. */
export interface RunnerNameTagProps {
  /** The raw identity, e.g. "@worker/7/claude_1". */
  identity: string;
  className?: string;
}

/**
 * A runner's friendly name followed by its worker id, "Michael (claude_1)",
 * with the raw identity in its tooltip and a family badge when another family
 * has a runner of the same name.
 */
export function RunnerNameTag({ identity, className }: RunnerNameTagProps) {
  const { detail, title, badge } = useRunnerLabel(identity);
  return (
    <span className={["runner-name", className].filter(Boolean).join(" ")} title={title}>
      {detail}
      {badge !== "" && <span className="runner-name__family">{badge}</span>}
    </span>
  );
}
