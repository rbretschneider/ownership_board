import { busiestDay, SEGMENTS, shortDay, type ThroughputDay } from "../../lib/stats";

/** Props for {@link ThroughputChart}. */
export interface ThroughputChartProps {
  days: ThroughputDay[];
}

/** Merges per day, stacked autonomous over the owner's; hover a day for its numbers. */
export function ThroughputChart({ days }: ThroughputChartProps) {
  const scale = busiestDay(days);
  const first = days[0];
  const last = days[days.length - 1];
  return (
    <figure className="throughput">
      <ul className="throughput__legend" aria-hidden="true">
        {SEGMENTS.map((s) => (
          <li key={s.key}><span className={`throughput__swatch throughput__swatch--${s.key}`} />{s.label}</li>
        ))}
      </ul>
      <div className="throughput__plot">
        <span className="throughput__max" aria-hidden="true">{scale}</span>
        <ol className="throughput__bars">
          {days.map((day) => {
            const label = `${shortDay(day.date)}: ${day.autonomous} autonomous, ${day.owner} yours merged`;
            return (
              <li key={day.date} className="throughput__day" title={label} aria-label={label}>
                <span className="throughput__stack" style={{ height: `${((day.autonomous + day.owner) / scale) * 100}%` }}>
                  {day.autonomous > 0 && <span className="throughput__seg throughput__seg--autonomous" style={{ flexGrow: day.autonomous }} />}
                  {day.owner > 0 && <span className="throughput__seg throughput__seg--owner" style={{ flexGrow: day.owner }} />}
                </span>
              </li>
            );
          })}
        </ol>
      </div>
      {first !== undefined && last !== undefined && (
        <figcaption className="throughput__axis">
          <span>{shortDay(first.date)}</span>
          <span>{shortDay(last.date)}</span>
        </figcaption>
      )}
    </figure>
  );
}
