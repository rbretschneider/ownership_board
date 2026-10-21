import { Bot, Hand, Replace, Trash2, UserCheck } from "lucide-react";
import type { Decision, DecisionWeight, Question } from "../../api/types";
import { decisionRound, replacementSummary, WEIGHT_LABELS } from "../../lib/decisions";
import { relativeTime } from "../../lib/relativeTime";
import { RowMenu } from "../common/RowMenu";

const WEIGHT_ICONS: Record<DecisionWeight, typeof Bot> = { binding: UserCheck, delegated: Hand, agent: Bot };

/** Props for {@link DecisionRow}. */
export interface DecisionRowProps {
  decision: Decision;
  all: readonly Decision[];
  questions: readonly Question[];
  /** Set for active decisions: the person may replace or delete them. */
  onReplace?: () => void;
  onDelete?: () => void;
}

/**
 * One decision. Every text field is shown as plain text, never rendered as
 * markdown or HTML: agents write some of it.
 */
export function DecisionRow({ decision, all, questions, onReplace, onDelete }: DecisionRowProps) {
  if (decision.is_removed) {
    return (
      <li className="decision decision--removed">
        <span className="muted">Removed</span> · <time dateTime={decision.at}>{relativeTime(decision.at)}</time>
      </li>
    );
  }
  const isHistory = decision.superseded_by_id !== null;
  const replacement = replacementSummary(decision, all);
  const round = decisionRound(decision, questions);
  const weight = WEIGHT_LABELS[decision.weight];
  const WeightIcon = WEIGHT_ICONS[decision.weight];
  const menuItems = [
    ...(onReplace !== undefined ? [{ label: "Replace…", icon: Replace, onSelect: onReplace }] : []),
    ...(onDelete !== undefined ? [{ label: "Delete", icon: Trash2, onSelect: onDelete, isDanger: true }] : []),
  ];

  return (
    <li className={`decision decision--${decision.weight}${isHistory ? " decision--superseded" : ""}`}>
      <div className="decision__head">
        <span className={`weight weight--${decision.weight}`} title={weight.title}>
          <WeightIcon size={13} aria-hidden="true" />
          {weight.text}
        </span>
        <p className="decision__summary">{decision.summary}</p>
        {menuItems.length > 0 && <RowMenu label={`decision “${decision.summary}”`} items={menuItems} />}
      </div>
      {decision.rationale !== "" && <p className="decision__rationale">{decision.rationale}</p>}
      {decision.alternatives.length > 0 && (
        <p className="decision__alternatives">Rejected: {decision.alternatives.join(" · ")}</p>
      )}
      <p className="decision__meta">
        decided by {decision.decided_by.name} · <time dateTime={decision.at} title={new Date(decision.at).toLocaleString()}>{relativeTime(decision.at)}</time>
        {decision.source === "answer" && round !== null && <> · from question round {round}</>}
        {decision.source === "answer" && decision.asked_by !== "" && <> · asked by {decision.asked_by}</>}
      </p>
      {isHistory && (
        <p className="decision__replaced-by">{replacement !== null ? `Replaced by “${replacement}”` : "Replaced"}</p>
      )}
    </li>
  );
}
