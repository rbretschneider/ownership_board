import type { LaneOption } from "../../lib/flow";

/** Props for {@link FlowFilters}. */
export interface FlowFiltersProps {
  options: LaneOption[];
  hidden: ReadonlySet<string>;
  onToggle: (key: string) => void;
  /** Shows one repo alone, or every repo with null. */
  onShowOnly: (key: string | null) => void;
  isDetailed: boolean;
  onDetailedChange: (value: boolean) => void;
  showCritical: boolean;
  onShowCriticalChange: (value: boolean) => void;
}

/** Repo chips (click toggles, double-click shows only that repo) and view switches. */
export function FlowFilters(props: FlowFiltersProps) {
  const { options, hidden } = props;
  return (
    <div className="flow-filters">
      <div className="flow-filters__repos" role="group" aria-label="Repositories">
        <button
          type="button"
          className={`chip chip--small${hidden.size === 0 ? " chip--on" : ""}`}
          aria-pressed={hidden.size === 0}
          onClick={() => props.onShowOnly(null)}
        >
          All repos
        </button>
        {options.map((o) => {
          const isShown = !hidden.has(o.key);
          return (
            <button
              key={o.key}
              type="button"
              className={`chip chip--small${isShown ? " chip--on" : ""}`}
              aria-pressed={isShown}
              onClick={() => props.onToggle(o.key)}
              onDoubleClick={() => props.onShowOnly(o.key)}
              title={`${isShown ? "Hide" : "Show"} ${o.name}. Double-click to show only ${o.name}.`}
            >
              <span className="flow-lane__dot" style={o.color === "" ? undefined : { background: o.color }} aria-hidden="true" />
              {o.name}
              <span className="chip__count">{o.open}</span>
            </button>
          );
        })}
      </div>
      <div className="flow-filters__switches">
        <label className="flow-toggle">
          <input type="checkbox" checked={props.isDetailed} onChange={(e) => props.onDetailedChange(e.target.checked)} />
          Show stages
        </label>
        <label className="flow-toggle">
          <input type="checkbox" checked={props.showCritical} onChange={(e) => props.onShowCriticalChange(e.target.checked)} />
          Critical paths
        </label>
      </div>
    </div>
  );
}
