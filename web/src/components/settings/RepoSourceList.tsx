import { CircleAlert, CircleCheck, CircleDashed } from "lucide-react";
import type { RepoSource } from "../../api/types";
import { relativeTime } from "../../lib/relativeTime";

/** Each provider's configuration and last sync result. */
export function RepoSourceList({ sources }: { sources: RepoSource[] }) {
  return (
    <ul className="source-list">
      {sources.map((source) => (
        <li key={source.provider} className="source">
          {sourceIcon(source)}
          <div className="source__text">
            <span className="source__label">{source.label}</span>
            <span className="muted small">{describe(source)}</span>
            {source.last_error !== "" && <span className="source__error">{source.last_error}</span>}
          </div>
        </li>
      ))}
    </ul>
  );
}

function sourceIcon(source: RepoSource) {
  if (!source.is_configured) {
    return <CircleDashed size={18} className="source__icon source__icon--off" aria-hidden="true" />;
  }
  if (source.last_error !== "") {
    return <CircleAlert size={18} className="source__icon source__icon--error" aria-hidden="true" />;
  }
  return <CircleCheck size={18} className="source__icon source__icon--ok" aria-hidden="true" />;
}

function describe(source: RepoSource): string {
  if (!source.is_configured) {
    return "Not configured (set on the server)";
  }
  const count = `${source.repo_count} ${source.repo_count === 1 ? "repo" : "repos"}`;
  const synced = source.last_synced_at === null ? "never synced" : `synced ${relativeTime(source.last_synced_at)}`;
  return `${count} · ${synced}`;
}
