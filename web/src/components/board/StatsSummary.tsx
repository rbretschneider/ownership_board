import { useStats } from "../../hooks/useStats";
import { STATS_HASH } from "../../lib/hashRoute";
import { formatMedian, SUMMARY_PERIOD, SUMMARY_REFRESH_MS } from "../../lib/stats";

/** Compact delivery stats for the top bar; each one opens the stats page. */
export function StatsSummary() {
  const { report, error } = useStats(SUMMARY_PERIOD, SUMMARY_REFRESH_MS);
  if (report === null) {
    return error === null ? null : (
      <a href={STATS_HASH} className="stats-summary__item stats-summary__item--muted" title={`Couldn't load stats: ${error}`}>
        Stats unavailable
      </a>
    );
  }
  const { all, autonomous, owner } = report;
  const split = (a: number, o: number) => `Autonomous ${a} · yours ${o}`;
  const items = [
    { value: String(all.merged_24h), label: "merged · 24h", title: split(autonomous.merged_24h, owner.merged_24h) },
    { value: String(all.merged), label: `merged · ${SUMMARY_PERIOD}d`, title: split(autonomous.merged, owner.merged) },
    {
      value: formatMedian(all.cycle_time),
      label: `cycle · ${SUMMARY_PERIOD}d`,
      title: "Median time from first in progress to merged",
    },
  ];
  return (
    <div className="stats-summary" role="group" aria-label="Delivery summary">
      {items.map((item) => (
        <a key={item.label} href={STATS_HASH} className="stats-summary__item" title={`${item.title}. Open stats.`}>
          <span className="stats-summary__value">{item.value}</span>
          <span className="stats-summary__label">{item.label}</span>
        </a>
      ))}
    </div>
  );
}
