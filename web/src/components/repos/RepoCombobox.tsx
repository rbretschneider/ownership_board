import { Check, ChevronDown, Search } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import type { RepoFilter } from "../../api/boardApi";
import type { Repo } from "../../api/types";
import { repoColor, searchRepos } from "../../lib/repos";
import { ProviderGlyph } from "./ProviderGlyph";

/** Props for {@link RepoCombobox}. */
export interface RepoComboboxProps {
  repos: Repo[];
  value: RepoFilter;
  onChange: (value: RepoFilter) => void;
  /** Visible label text (hidden visually when `isLabelHidden`). */
  label: string;
  isLabelHidden?: boolean;
  /** Offer "All repos" (board filter only). */
  includeAll?: boolean;
  /** Text for the no-repo option. */
  noneLabel: string;
  /** Extra id for `aria-describedby` on the trigger. */
  describedBy?: string;
  isDisabled?: boolean;
  className?: string;
}

interface Option {
  key: string;
  value: RepoFilter;
  repo: Repo | null;
  text: string;
}

/**
 * A searchable repository picker following the ARIA combobox pattern: a button
 * opens a search box with a listbox; arrows move, Enter picks, Escape closes.
 */
export function RepoCombobox(props: RepoComboboxProps) {
  const { repos, value, onChange, includeAll = false, noneLabel } = props;
  const baseId = useId();
  const [isOpen, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const options = useMemo(() => buildOptions(repos, query, includeAll, noneLabel), [repos, query, includeAll, noneLabel]);
  const selected = describe(value, repos, noneLabel);

  useEffect(() => {
    if (!isOpen) {
      return;
    }
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current !== null && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [isOpen]);

  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  useEffect(() => {
    document.getElementById(`${baseId}-opt-${activeIndex}`)?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, baseId]);

  const open = () => {
    setQuery("");
    const current = options.findIndex((o) => sameValue(o.value, value));
    setActiveIndex(Math.max(0, current));
    setOpen(true);
  };

  const close = (shouldRefocus: boolean) => {
    setOpen(false);
    if (shouldRefocus) {
      triggerRef.current?.focus();
    }
  };

  const pick = (option: Option) => {
    onChange(option.value);
    close(true);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    const handled = handleKey(event.key, options.length, activeIndex, setActiveIndex);
    if (event.key === "Enter") {
      const option = options[activeIndex];
      if (option !== undefined) {
        pick(option);
      }
    } else if (event.key === "Escape") {
      // Keep the surrounding dialog open; only the popover closes.
      event.stopPropagation();
      close(true);
    } else if (event.key === "Tab") {
      close(false);
    }
    if (handled || event.key === "Enter" || event.key === "Escape") {
      event.preventDefault();
    }
  };

  const labelId = `${baseId}-label`;
  const listId = `${baseId}-list`;

  return (
    <div className={`combo ${props.className ?? ""}`} ref={rootRef}>
      <span id={labelId} className={props.isLabelHidden === true ? "visually-hidden" : "field__label"}>{props.label}</span>
      <button
        ref={triggerRef}
        type="button"
        className="input combo__trigger"
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-labelledby={`${labelId} ${baseId}-value`}
        aria-describedby={props.describedBy}
        disabled={props.isDisabled}
        onClick={() => (isOpen ? close(false) : open())}
      >
        {selected.repo !== null && <span className="repo-chip__dot" style={{ background: repoColor(selected.repo) }} aria-hidden="true" />}
        <span id={`${baseId}-value`} className="combo__value">{selected.text}</span>
        <ChevronDown size={16} aria-hidden="true" />
      </button>
      {isOpen && (
        <div className="combo__popover">
          <div className="combo__search">
            <Search size={16} aria-hidden="true" />
            <input
              className="combo__input"
              role="combobox"
              aria-label={`Search ${props.label.toLowerCase()}`}
              aria-expanded="true"
              aria-controls={listId}
              aria-autocomplete="list"
              aria-activedescendant={options.length > 0 ? `${baseId}-opt-${activeIndex}` : undefined}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={onKeyDown}
              placeholder="Search repos"
              autoFocus
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <ul id={listId} className="combo__list" role="listbox" aria-labelledby={labelId}>
            {options.map((option, index) => (
              <li
                key={option.key}
                id={`${baseId}-opt-${index}`}
                role="option"
                aria-selected={sameValue(option.value, value)}
                className={`combo__option${index === activeIndex ? " combo__option--active" : ""}`}
                onPointerMove={() => setActiveIndex(index)}
                onClick={() => pick(option)}
              >
                {option.repo !== null ? <ProviderGlyph provider={option.repo.provider} /> : <span className="provider provider--none" aria-hidden="true" />}
                {option.repo !== null && <span className="repo-chip__dot" style={{ background: repoColor(option.repo) }} aria-hidden="true" />}
                <span className="combo__option-text">{option.text}</span>
                {option.repo?.is_archived === true && <span className="badge">archived</span>}
                {sameValue(option.value, value) && <Check size={16} aria-hidden="true" />}
              </li>
            ))}
            {options.length === 0 && <li className="combo__empty" role="presentation">No repos match “{query}”.</li>}
          </ul>
        </div>
      )}
    </div>
  );
}

function buildOptions(repos: Repo[], query: string, includeAll: boolean, noneLabel: string): Option[] {
  const fixed: Option[] = [];
  if (query.trim() === "") {
    if (includeAll) {
      fixed.push({ key: "all", value: { kind: "all" }, repo: null, text: "All repos" });
    }
    fixed.push({ key: "none", value: { kind: "none" }, repo: null, text: noneLabel });
  }
  const matches = searchRepos(repos, query).map<Option>((repo) => ({
    key: `repo-${repo.id}`,
    value: { kind: "repo", id: repo.id },
    repo,
    text: repo.full_name,
  }));
  return [...fixed, ...matches];
}

function describe(value: RepoFilter, repos: Repo[], noneLabel: string): { text: string; repo: Repo | null } {
  if (value.kind === "all") {
    return { text: "All repos", repo: null };
  }
  if (value.kind === "none") {
    return { text: noneLabel, repo: null };
  }
  const repo = repos.find((r) => r.id === value.id) ?? null;
  return { text: repo?.full_name ?? "Unknown repo", repo };
}

function sameValue(a: RepoFilter, b: RepoFilter): boolean {
  return a.kind === b.kind && (a.kind !== "repo" || (b.kind === "repo" && a.id === b.id));
}

/** Arrow-key navigation (Home/End stay with the text input). Returns true when handled. */
function handleKey(key: string, count: number, active: number, setActive: (index: number) => void): boolean {
  if (count === 0) {
    return false;
  }
  const moves: Record<string, number> = {
    ArrowDown: Math.min(count - 1, active + 1),
    ArrowUp: Math.max(0, active - 1),
  };
  const next = moves[key];
  if (next === undefined) {
    return false;
  }
  setActive(next);
  return true;
}
