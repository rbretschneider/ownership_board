import type { EpicProgress } from "../../lib/epic";

const STATE_TEXT = { deployed: "deployed", merged: "merged", active: "in progress", waiting: "not started" } as const;

/**
 * An epic's stories as one segmented bar along the card's bottom edge, in
 * build order, in place of the stage stepper: an epic is never worked
 * itself, so its progress is its stories' progress.
 */
export function EpicProgressBar({ progress }: { progress: EpicProgress }) {
  return (
    <div className="epic-bar" role="img" aria-label={`Epic: ${progress.label}`} title={`Epic: ${progress.label}`}>
      <ol className="epic-bar__parts" aria-hidden="true">
        {progress.parts.map(({ child, state }) => (
          <li key={child.id} className={`epic-bar__part epic-bar__part--${state}`} title={`#${child.id} ${child.title}: ${STATE_TEXT[state]}`} />
        ))}
      </ol>
      <span className="epic-bar__label" aria-hidden="true">{progress.label}</span>
    </div>
  );
}
