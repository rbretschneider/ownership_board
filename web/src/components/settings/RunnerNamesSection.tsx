import { useId } from "react";
import { useRunnerNames } from "../../hooks/useRunnerNames";
import { RunnerNameRow } from "./RunnerNameRow";

/**
 * Runner names: each worker gets a name from its family's pool the first time
 * it claims work, and keeps it. The owner may rename one here.
 */
export function RunnerNamesSection() {
  const id = useId();
  const { directory, refresh } = useRunnerNames();
  const runners = [...directory.values()];
  return (
    <section className="settings-section" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="settings-section__title">Runner names</h2>
      <p className="muted small">
        Each runner takes a name from its family&apos;s pool the first time it claims work, and keeps it across restarts.
        The raw worker id stays in tooltips and the API.
      </p>
      {runners.length === 0
        ? <p className="muted small" role="status">No runner has claimed work yet.</p>
        : runners.map((runner) => <RunnerNameRow key={runner.worker} runner={runner} onRenamed={refresh} />)}
    </section>
  );
}
