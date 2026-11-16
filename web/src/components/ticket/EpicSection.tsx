import type { Repo, Stage, TicketDetail } from "../../api/types";
import { epicProgress } from "../../lib/epic";
import { ticketHash } from "../../lib/hashRoute";
import { EpicProgressBar } from "../board/EpicProgressBar";
import { StageChip } from "../common/StageChip";

/**
 * The stories a ticket was split into, in build order, with where each one
 * stands. An epic closes on its own once every story is deployed.
 */
export function EpicSection({ ticket, stages, repo }: { ticket: TicketDetail; stages: Stage[]; repo?: Repo }) {
  const progress = epicProgress(ticket);
  if (progress === null) return null;
  const isEpic = ticket.stage === "epic";
  return (
    <section className="section" aria-labelledby="epic-title">
      <div className="section__head">
        <h3 id="epic-title" className="section__title">Stories</h3>
        <span className="section__hint">{progress.label}</span>
      </div>
      <p className="muted">
        {isEpic
          ? "This request was split into these stories, built in this order. It closes on its own when every story is deployed."
          : ticket.stage === "deployed" && ticket.split_into?.every((c) => c.stage === "deployed")
            ? "Every story is deployed."
            : "Work moved out of this ticket into these stories; they wait for it."}
      </p>
      <div className="epic-section__bar">
        <EpicProgressBar progress={progress} />
      </div>
      <ol className="related-list epic-section__list">
        {progress.parts.map(({ child }) => (
          <li key={child.id} className="related-item">
            <a className="related-item__link" href={ticketHash(child.id)}>
              <span className="related-item__id">#{child.id}</span> {child.title}
            </a>
            <StageChip stage={child.stage} stages={stages} repo={repo} />
          </li>
        ))}
      </ol>
    </section>
  );
}
