import type { Lane, Stage, TimelineEntry } from "../../api/types";
import { TimelineItem } from "./TimelineItem";

/** The ticket's history, oldest first. */
export function Timeline({ entries, lanes, stages }: { entries: TimelineEntry[]; lanes: Lane[]; stages: Stage[] }) {
  if (entries.length === 0) {
    return null;
  }
  return (
    <section className="section" aria-labelledby="timeline-title">
      <h3 id="timeline-title" className="section__title">Activity</h3>
      <ol className="timeline">
        {entries.map((entry) => (
          <TimelineItem key={entry.id} entry={entry} lanes={lanes} stages={stages} />
        ))}
      </ol>
    </section>
  );
}
