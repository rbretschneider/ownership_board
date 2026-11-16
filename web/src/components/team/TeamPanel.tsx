import { ChevronDown, X } from "lucide-react";
import { useEffect, useId, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { clockTime } from "../../lib/team";
import { Markdown } from "../common/Markdown";

/** One fact in the panel's grid. */
export interface PanelFact {
  label: string;
  value: string;
}

/** One line of a runner's recent activity. */
export interface PanelLogLine {
  at: string;
  text: string;
  tag: string;
  tone: "accent" | "green" | "amber" | "grey";
}

/** One manager session in the history log. */
export interface PanelHistoryEntry {
  at: string;
  headline: string;
  /** Cost and model, shown as a neutral chip. */
  chip: string;
  body: string;
  isMarkdown: boolean;
}

/** What the modal shows. */
export interface PanelContent {
  kind: string;
  title: string;
  summary: string;
  facts: PanelFact[];
  logTitle: string;
  log: PanelLogLine[];
  /** The manager's session history, newest first; replaces `log` when set. */
  history?: PanelHistoryEntry[];
  /** Shown instead of the log while it loads or when it is empty. */
  logNote?: string;
}

/** Props for {@link TeamPanel}. */
export interface TeamPanelProps {
  content: PanelContent;
  onClose: () => void;
}

const FOCUSABLE = 'a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])';

/**
 * The details modal: centered over a dimmed backdrop, a full-screen sheet on a
 * phone. Closes on ×, a click on the backdrop, or Escape; focus stays inside
 * while it is open and returns to where it was when it closes.
 */
export function TeamPanel({ content, onClose }: TeamPanelProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeRef.current?.focus();
    return () => previous?.focus();
  }, []);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Tab and Shift+Tab cycle within the modal.
  const trapFocus = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Tab" || dialogRef.current === null) return;
    const items = [...dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE)];
    const first = items[0];
    const last = items[items.length - 1];
    if (first === undefined || last === undefined) return;
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  return (
    <div className="team-modal" data-testid="team-backdrop" onClick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <div ref={dialogRef} className="team-modal__dialog" role="dialog" aria-modal="true" aria-labelledby={titleId} aria-label={content.title} onKeyDown={trapFocus}>
        <div className="team-row team-row--between team-row--top">
          <div>
            <div className="team-faint">{content.kind}</div>
            <h2 className="team-panel__title" id={titleId}>{content.title}</h2>
          </div>
          <button ref={closeRef} type="button" className="team-panel__close" onClick={onClose} aria-label="Close details">
            <X size={18} aria-hidden="true" />
          </button>
        </div>
        {content.summary !== "" && <p className="team-panel__summary">{content.summary}</p>}
        <dl className="team-facts">
          {content.facts.map((fact) => (
            <div key={fact.label} className="team-fact">
              <dt>{fact.label}</dt>
              <dd>{fact.value}</dd>
            </div>
          ))}
        </dl>
        <div>
          <h3 className="team-heading">{content.logTitle}</h3>
          {content.logNote !== undefined && <p className="team-faint">{content.logNote}</p>}
          {content.logNote === undefined && content.history !== undefined && (
            <ol className="team-history">
              {content.history.map((entry, i) => <HistoryItem key={`${entry.at}-${i}`} entry={entry} isOpenAtFirst={i === 0} />)}
            </ol>
          )}
          {content.logNote === undefined && content.history === undefined && (
            <ol className="team-log">
              {content.log.map((line, i) => (
                <li key={`${line.at}-${i}`} className="team-log__item">
                  <time className="team-log__at" dateTime={line.at}>{clockTime(line.at)}</time>
                  <span className="team-log__body">
                    <span className="team-log__text">{line.text}</span>
                    <span className={`team-log__tag team-tone--${line.tone}`}>{line.tag}</span>
                  </span>
                </li>
              ))}
            </ol>
          )}
        </div>
      </div>
    </div>
  );
}

/** One manager session: time, bold headline, a cost/model chip, and its details behind a toggle. */
function HistoryItem({ entry, isOpenAtFirst }: { entry: PanelHistoryEntry; isOpenAtFirst: boolean }) {
  const [isOpen, setOpen] = useState(isOpenAtFirst);
  const bodyId = useId();
  const hasBody = entry.body !== "";
  return (
    <li className="team-history__item">
      <div className="team-history__head">
        <time className="team-log__at" dateTime={entry.at}>{clockTime(entry.at)}</time>
        <strong className="team-history__headline">{entry.headline || "No headline recorded"}</strong>
        {entry.chip !== "" && <span className="team-chip">{entry.chip}</span>}
      </div>
      {hasBody && (
        <>
          <button type="button" className="team-history__toggle" aria-expanded={isOpen} aria-controls={bodyId} onClick={() => setOpen((v) => !v)}>
            <ChevronDown size={14} aria-hidden="true" className={isOpen ? "team-history__chevron--open" : undefined} />
            {isOpen ? "Hide details" : "Details"}
          </button>
          {isOpen && (
            <div id={bodyId} className="team-history__body">
              {entry.isMarkdown ? <Markdown source={entry.body} /> : <p className="team-history__plain">{entry.body}</p>}
            </div>
          )}
        </>
      )}
    </li>
  );
}
