import { CloudOff, Plus, Settings } from "lucide-react";
import type { RepoFilter } from "../../api/boardApi";
import type { Repo } from "../../api/types";
import { SETTINGS_HASH } from "../../lib/hashRoute";
import { BrandMark } from "../common/BrandMark";
import { RepoCombobox } from "../repos/RepoCombobox";

/** Props for {@link TopBar}. */
export interface TopBarProps {
  repos: Repo[];
  repoFilter: RepoFilter;
  onRepoFilterChange: (filter: RepoFilter) => void;
  yourMoveCount: number;
  isYourMoveOnly: boolean;
  onToggleYourMove: () => void;
  isConnected: boolean;
  /** Shows the inline "New" button (desktop); phones use the floating button. */
  showNewButton: boolean;
  onNew: () => void;
}

/** App header: brand, repo filter, "your move" filter, settings. */
export function TopBar(props: TopBarProps) {
  const { yourMoveCount, isYourMoveOnly } = props;
  return (
    <header className="topbar">
      <a href="#/" className="brand" aria-label="ownership board home">
        <BrandMark />
        <span className="brand__name">ownership</span>
      </a>
      <RepoCombobox
        className="topbar__repo"
        repos={props.repos}
        value={props.repoFilter}
        onChange={props.onRepoFilterChange}
        label="Repository filter"
        isLabelHidden
        includeAll
        noneLabel="No repo"
      />
      <button
        type="button"
        className={`chip${isYourMoveOnly ? " chip--active" : ""}${yourMoveCount > 0 ? " chip--alert" : ""}`}
        aria-pressed={isYourMoveOnly}
        onClick={props.onToggleYourMove}
      >
        Your move <span className="chip__count">{yourMoveCount}</span>
      </button>
      <span className="topbar__spacer" />
      {!props.isConnected && (
        <span className="offline" role="status" title="Live updates paused; reconnecting">
          <CloudOff size={16} aria-hidden="true" />
          <span className="offline__label">Offline</span>
        </span>
      )}
      {props.showNewButton && (
        <button type="button" className="button button--primary" onClick={props.onNew}>
          <Plus size={18} aria-hidden="true" />
          New
        </button>
      )}
      <a href={SETTINGS_HASH} className="icon-button" aria-label="Settings">
        <Settings size={20} aria-hidden="true" />
      </a>
    </header>
  );
}
