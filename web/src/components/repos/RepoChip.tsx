import type { Repo } from "../../api/types";
import { repoColor } from "../../lib/repos";

/** Repo name with its color dot, or a muted "no repo" chip. */
export function RepoChip({ repo, showNone = true }: { repo: Repo | undefined; showNone?: boolean }) {
  if (repo === undefined) {
    return showNone ? <span className="repo-chip repo-chip--none">no repo</span> : null;
  }
  return (
    <span className="repo-chip" title={repo.full_name}>
      <span className="repo-chip__dot" style={{ background: repoColor(repo) }} aria-hidden="true" />
      {repo.name}
    </span>
  );
}
