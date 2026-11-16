import { Coins } from "lucide-react";
import type { TokenUsage } from "../../api/types";
import { usageBadge } from "../../lib/usage";

/** A ticket's reported token usage, flagged when heavy; nothing when none was reported. */
export function UsageBadge({ usage }: { usage: TokenUsage | undefined }) {
  const badge = usageBadge(usage);
  if (!badge) return null;
  return (
    <span className={`badge badge--usage${badge.heavy ? " badge--usage-heavy" : ""}`} title={badge.title} aria-label={badge.title}>
      <Coins size={13} aria-hidden="true" />
      {badge.label}
    </span>
  );
}

/** Three stacked chevrons, like rank stripes: the story earned a stronger model. */
export function RankChevrons() {
  return (
    <svg className="rank-chevrons" width="11" height="12" viewBox="0 0 11 12" aria-hidden="true">
      <path d="M1.5 4.5 5.5 1.5 9.5 4.5M1.5 7.5 5.5 4.5 9.5 7.5M1.5 10.5 5.5 7.5 9.5 10.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
