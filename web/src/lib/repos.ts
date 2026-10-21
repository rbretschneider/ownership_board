import type { RepoFilter } from "../api/boardApi";
import type { Provider, Repo } from "../api/types";

/** Display names for repository providers. */
export const PROVIDER_LABELS: Record<Provider, string> = {
  github: "GitHub",
  bitbucket: "Bitbucket",
  forgejo: "Forgejo",
  manual: "Manual",
};

const FILTER_NONE = "none";
const HUE_DEGREES = 360;
const FALLBACK_SATURATION = 62;
const FALLBACK_LIGHTNESS = 66;
/** Most matches a repo picker shows at once. */
export const MAX_REPO_RESULTS = 50;

/** Encodes a filter for storage: "" = all, "none", or the repo id. */
export function encodeRepoFilter(filter: RepoFilter): string {
  switch (filter.kind) {
    case "all":
      return "";
    case "none":
      return FILTER_NONE;
    case "repo":
      return String(filter.id);
  }
}

/** Decodes {@link encodeRepoFilter} output; anything unrecognised means "all". */
export function decodeRepoFilter(value: string): RepoFilter {
  if (value === FILTER_NONE) {
    return { kind: "none" };
  }
  const id = Number(value);
  return value !== "" && Number.isInteger(id) && id > 0 ? { kind: "repo", id } : { kind: "all" };
}

/** The repo's color, or a stable hue derived from its full name. */
export function repoColor(repo: Pick<Repo, "color" | "full_name">): string {
  if (repo.color !== "") {
    return repo.color;
  }
  let hash = 0;
  for (const char of repo.full_name) {
    hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  }
  return `hsl(${hash % HUE_DEGREES} ${FALLBACK_SATURATION}% ${FALLBACK_LIGHTNESS}%)`;
}

/**
 * Filters repos for a picker. Matching is case-insensitive on `full_name`;
 * archived repos, and repos the provider no longer lists, only appear when the
 * query names them exactly. (A picker shows its current value separately, so
 * an already-selected hidden repo still displays.)
 * Prefix matches on the short name sort first.
 */
export function searchRepos(repos: readonly Repo[], query: string): Repo[] {
  const needle = query.trim().toLowerCase();
  const visible = repos.filter((repo) => isVisible(repo, needle));
  if (needle === "") {
    return visible.slice(0, MAX_REPO_RESULTS);
  }
  return visible
    .filter((repo) => repo.full_name.toLowerCase().includes(needle))
    .sort((a, b) => rank(a, needle) - rank(b, needle) || a.full_name.localeCompare(b.full_name))
    .slice(0, MAX_REPO_RESULTS);
}

function isVisible(repo: Repo, needle: string): boolean {
  const isHidden = repo.is_archived || repo.missing_since !== null;
  return !isHidden || repo.full_name.toLowerCase() === needle;
}

function rank(repo: Repo, needle: string): number {
  return repo.name.toLowerCase().startsWith(needle) ? 0 : 1;
}
