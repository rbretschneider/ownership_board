import { X } from "lucide-react";
import { useEffect, useRef } from "react";
import { clockTime } from "../../lib/team";

/** One fact in the panel's grid. */
export interface PanelFact {
  label: string;
  value: string;
}

/** One line of the panel's log. */
export interface PanelLogLine {
  at: string;
  text: string;
  tag: string;
  tone: "accent" | "green" | "amber" | "grey";
}

/** What the right-hand panel shows. */
export interface PanelContent {
  kind: string;
  title: string;
  summary: string;
  facts: PanelFact[];
  logTitle: string;
  log: PanelLogLine[];
  /** Shown instead of the log while it loads or when it is empty. */
  logNote?: string;
}

/** Props for {@link TeamPanel}. */
export interface TeamPanelProps {
  content: PanelContent;
  onClose: () => void;
}

/** The right-hand detail panel. Closes on ×, a tap outside, or Escape. */
export function TeamPanel({ content, onClose }: TeamPanelProps) {
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    closeRef.current?.focus();
  }, []);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <>
      <div className="team-backdrop" onClick={onClose} data-testid="team-backdrop" />
      <aside className="team-panel" role="dialog" aria-modal="true" aria-label={content.title}>
        <div className="team-row team-row--between team-row--top">
          <div>
            <div className="team-faint">{content.kind}</div>
            <h2 className="team-panel__title">{content.title}</h2>
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
          {content.logNote !== undefined
            ? <p className="team-faint">{content.logNote}</p>
            : (
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
      </aside>
    </>
  );
}
