import { ArrowRightLeft, Bot, CirclePlus, FileText, Hand, MessageCircleQuestion, MessageSquareReply, Pencil, Settings2, Unlock, User } from "lucide-react";
import type { ActorKind, Lane, TimelineEntry, TimelineType } from "../../api/types";
import { relativeTime } from "../../lib/relativeTime";
import { Markdown } from "../common/Markdown";

const EVENT_ICONS: Record<Exclude<TimelineType, "comment">, typeof Bot> = {
  created: CirclePlus,
  moved: ArrowRightLeft,
  spec: FileText,
  questions: MessageCircleQuestion,
  answers: MessageSquareReply,
  edited: Pencil,
  claimed: Hand,
  released: Unlock,
};

const ACTOR_ICONS: Record<ActorKind, typeof Bot> = { agent: Bot, human: User, system: Settings2 };

/** One timeline entry: a comment bubble or a compact event line. */
export function TimelineItem({ entry, lanes }: { entry: TimelineEntry; lanes: Lane[] }) {
  const ActorIcon = ACTOR_ICONS[entry.actor.kind];
  const when = <time dateTime={entry.at} title={new Date(entry.at).toLocaleString()}>{relativeTime(entry.at)}</time>;

  if (entry.type === "comment") {
    return (
      <li className={`comment comment--${entry.actor.kind}`}>
        <div className="comment__meta">
          <ActorIcon size={14} aria-hidden="true" />
          <span className="comment__author">{entry.actor.name}</span>
          {entry.actor.kind === "agent" && <span className="visually-hidden">(AI)</span>}
          {when}
        </div>
        <div className="comment__body"><Markdown source={entry.body} /></div>
      </li>
    );
  }

  const EventIcon = EVENT_ICONS[entry.type];
  return (
    <li className={`event event--${entry.actor.kind}`}>
      <EventIcon size={14} aria-hidden="true" />
      <span className="event__text">
        <strong>{entry.actor.name}</strong> {describe(entry, lanes)} · {when}
        {entry.type === "questions" && entry.body !== "" && <span className="event__detail">{entry.body}</span>}
      </span>
    </li>
  );
}

function describe(entry: TimelineEntry, lanes: Lane[]): string {
  const label = (id: unknown) => lanes.find((l) => l.id === id)?.label ?? String(id);
  const count = typeof entry.data["count"] === "number" ? entry.data["count"] : 0;
  const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;
  switch (entry.type) {
    case "created":
      return "created this";
    case "moved":
      return `moved this from ${label(entry.data["from"])} to ${label(entry.data["to"])}`;
    case "spec":
      return "updated the spec";
    case "questions":
      return `asked ${plural(count, "question")}`;
    case "answers":
      return `answered ${plural(count, "question")}`;
    case "edited":
      return `edited ${Array.isArray(entry.data["fields"]) ? entry.data["fields"].join(", ") : "this"}`;
    case "claimed":
      return typeof entry.data["action"] === "string" ? `claimed this to ${entry.data["action"]}` : "claimed this";
    case "released":
      return "released this";
    default:
      return entry.type;
  }
}
