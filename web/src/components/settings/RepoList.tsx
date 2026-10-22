import { useState } from "react";
import type { Repo } from "../../api/types";
import { PROVIDER_LABELS, repoColor } from "../../lib/repos";
import { ProviderGlyph } from "../repos/ProviderGlyph";
import { RepoColorPicker } from "./RepoColorPicker";
import { RepoGateToggles } from "./RepoGateToggles";
import { RepoVisibilityToggle } from "./RepoVisibilityToggle";

const SEARCH_ID = "repo-list-search";

/** Searchable list of every repo, with per-repo color, visibility and checkpoint settings. */
export function RepoList({ repos, onUpdated }: { repos: Repo[]; onUpdated: (repo: Repo) => void }) {
  const [query, setQuery] = useState("");
  const [editingId, setEditingId] = useState<number | null>(null);
  const needle = query.trim().toLowerCase();
  const shown = needle === "" ? repos : repos.filter((r) => r.full_name.toLowerCase().includes(needle));

  if (repos.length === 0) {
    return <p className="muted">No repositories yet. Connect a provider above, or add one by hand.</p>;
  }
  return (
    <div className="repo-list">
      <label className="visually-hidden" htmlFor={SEARCH_ID}>Search repositories</label>
      <input
        id={SEARCH_ID}
        className="input"
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={`Search ${repos.length} repositories`}
        autoComplete="off"
      />
      <ul className="repo-list__items">
        {shown.map((repo) => (
          <li key={repo.id} className="repo-row">
            <div className="repo-row__main">
              <ProviderGlyph provider={repo.provider} />
              <button
                type="button"
                className="repo-row__swatch"
                style={{ background: repoColor(repo) }}
                aria-label={`Change color for ${repo.full_name}`}
                aria-expanded={editingId === repo.id}
                onClick={() => setEditingId(editingId === repo.id ? null : repo.id)}
              />
              <span className="repo-row__name">{repo.full_name}</span>
              {repo.is_archived && <span className="badge">archived</span>}
              {repo.missing_since !== null && <span className="muted small">No longer on {PROVIDER_LABELS[repo.provider]}</span>}
            </div>
            <RepoVisibilityToggle repo={repo} onUpdated={onUpdated} />
            <RepoGateToggles repo={repo} onUpdated={onUpdated} />
            {editingId === repo.id && (
              <RepoColorPicker
                repo={repo}
                onSaved={(updated) => {
                  onUpdated(updated);
                  setEditingId(null);
                }}
              />
            )}
          </li>
        ))}
        {shown.length === 0 && <li className="muted">No repositories match “{query}”.</li>}
      </ul>
    </div>
  );
}
