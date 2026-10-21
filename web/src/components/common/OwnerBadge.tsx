import { Bot, Check, User } from "lucide-react";
import type { Owner } from "../../api/types";
import { OWNER_LABELS } from "../../lib/constants";

const ICONS: Record<Owner, typeof Bot> = { agent: Bot, human: User, nobody: Check };

/** Who holds the ball, as icon plus text. */
export function OwnerBadge({ owner, label }: { owner: Owner; label?: string }) {
  const Icon = ICONS[owner];
  return (
    <span className={`owner owner--${owner}`}>
      <Icon size={13} aria-hidden="true" />
      {label ?? OWNER_LABELS[owner]}
    </span>
  );
}
