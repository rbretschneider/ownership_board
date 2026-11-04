import type { ReactNode } from "react";
import type { TicketSummary } from "../../api/types";
import { formatDays, type Callout, type FlowInsights, type FlowLane, type LaneForecast } from "../../lib/flow";
import { relativeTime } from "../../lib/relativeTime";

/** Props for {@link FlowInsightCards}. */
export interface FlowInsightCardsProps {
  insights: FlowInsights;
  lanes: FlowLane[];
  forecasts: Map<string, LaneForecast>;
  /** Median cycle time from GET /stats, or null before it loads. */
  cycleHours: number | null;
  now: number;
  /** Focuses a ticket on the canvas. */
  onSelect: (id: number) => void;
}

/**
 * The owner's questions, answered above the canvas: where is it stuck, what
 * can I unblock, is it moving, are tokens being wasted, and when will it land.
 */
export function FlowInsightCards({ insights, lanes, forecasts, cycleHours, now, onSelect }: FlowInsightCardsProps) {
  const { stuck, help, progress, waste } = insights;
  const stuckCount = stuck.reduce((sum, s) => sum + s.tickets.length, 0);
  const pick = (c: Callout) => <TicketPick key={c.ticket.id} ticket={c.ticket} note={c.note} tone={c.tone} onSelect={onSelect} />;
  const retryShare = waste.totalPasses > 0 ? Math.round((waste.totalRetries / waste.totalPasses) * 100) : 0;
  return (
    <section className="flow-glance" aria-label="At a glance">
      <GlanceCard
        title="Where is it stuck?"
        value={stuckCount}
        unit={stuck[0] === undefined ? "stuck" : `stuck · most in ${stuck[0].label}`}
        tone={stuckCount > 0 ? "stuck" : "working"}
        empty="Nothing is stalled: every started ticket has an agent or a Windows run on it."
      >
        {stuck.map((s) => (
          <li key={s.key} className="flow-glance__group">
            <span className="flow-glance__group-name">{s.label} · {s.tickets.length}</span>
            <ul className="flow-glance__list">{s.tickets.map(pick)}</ul>
          </li>
        ))}
      </GlanceCard>
      <GlanceCard
        title="What can you unblock?"
        value={help.length}
        unit={help.length === 1 ? "action" : "actions"}
        tone={help.some((h) => h.tone === "owner" || h.tone === "stuck") ? "owner" : undefined}
        empty="Nothing needs you right now."
      >
        {help.map(pick)}
      </GlanceCard>
      <GlanceCard
        title="Is it moving?"
        value={progress.merged24h}
        unit={`merged in 24 h · ${progress.merged7d} this week`}
        tone={progress.merged24h > 0 ? "working" : progress.working > 0 ? undefined : "stuck"}
      >
        <li className="flow-glance__fact"><strong>{progress.working}</strong> tickets being worked right now</li>
        <li className="flow-glance__fact">
          {progress.lastMergeAt === null ? "No merges yet" : <>Last merge <strong>{relativeTime(new Date(progress.lastMergeAt).toISOString(), new Date(now))}</strong></>}
        </li>
        {progress.idle > 0 && <li className="flow-glance__fact"><strong>{progress.idle}</strong> started but nobody on them</li>}
      </GlanceCard>
      <GlanceCard
        title="Tokens wasted?"
        value={waste.totalRetries}
        unit={waste.totalPasses > 0 ? `retries · ${retryShare}% of agent passes` : "retries"}
        tone={waste.hotSpots.some((h) => h.tone === "stuck") ? "stuck" : waste.hotSpots.length > 0 ? "warm" : "working"}
        empty="No loops: work is passing first or second time."
      >
        {waste.hotSpots.map(pick)}
      </GlanceCard>
      <GlanceCard
        title="When will it land?"
        value={lanes.length}
        unit={lanes.length === 1 ? "repo in flight" : "repos in flight"}
        isWide
        empty="No open work."
      >
        {lanes.map((lane) => <ForecastRow key={lane.key} lane={lane} forecast={forecasts.get(lane.key)} />)}
        <li className="flow-glance__footnote">
          Close rate is merges over the last 7 days. The chain floor works the longest dependency chain one ticket at a
          time at the median cycle time{cycleHours === null ? " (loading)" : ` (${formatDays(cycleHours / 24)})`}. The
          larger one binds. An estimate to steer by, not a promise.
        </li>
      </GlanceCard>
    </section>
  );
}

function ForecastRow({ lane, forecast }: { lane: FlowLane; forecast: LaneForecast | undefined }) {
  const done = lane.merged;
  const total = lane.merged + lane.open;
  const share = total > 0 ? (done / total) * 100 : 0;
  let verdict: ReactNode = "No merges this week; no estimate";
  if (forecast?.etaDays != null) {
    verdict = (
      <>
        ~<strong>{formatDays(forecast.etaDays)}</strong> ·{" "}
        {forecast.bound === "dependencies" ? "bound by the dependency chain" : "bound by close rate"}
      </>
    );
  }
  return (
    <li className="flow-forecast">
      <span className="flow-forecast__name">
        <span className="flow-lane__dot" style={lane.color === "" ? undefined : { background: lane.color }} aria-hidden="true" />
        {lane.name}
      </span>
      <span className="flow-forecast__bar" role="img" aria-label={`${done} merged this week, ${lane.open} open`}>
        <span className="flow-forecast__fill" style={{ width: `${share}%` }} />
      </span>
      <span className="flow-forecast__nums">
        {lane.open} open · {forecast === undefined ? "" : `${forecast.closeRate.toFixed(1)}/day`}
        {lane.criticalPath.length > 1 && ` · chain ${lane.criticalPath.length}`}
      </span>
      <span className="flow-forecast__eta">{verdict}</span>
    </li>
  );
}

interface GlanceCardProps {
  title: string;
  value: number;
  unit?: string;
  tone?: string;
  isWide?: boolean;
  /** Shown when the card has no rows. */
  empty?: string;
  children: ReactNode;
}

function GlanceCard({ title, value, unit, tone, isWide = false, empty, children }: GlanceCardProps) {
  const rows = Array.isArray(children) ? children.flat().filter(Boolean) : children;
  const isEmpty = Array.isArray(rows) && rows.length === 0;
  return (
    <article className={`flow-glance__card${tone === undefined ? "" : ` flow-glance__card--${tone}`}${isWide ? " flow-glance__card--wide" : ""}`}>
      <h2 className="flow-glance__title">{title}</h2>
      <p className="flow-glance__value">
        {value}
        {unit !== undefined && <span className="flow-glance__unit"> {unit}</span>}
      </p>
      {isEmpty
        ? <p className="flow-glance__empty">{empty}</p>
        : <ul className="flow-glance__list">{rows}</ul>}
    </article>
  );
}

function TicketPick({ ticket, note, tone, onSelect }: { ticket: TicketSummary; note: string; tone: string; onSelect: (id: number) => void }) {
  return (
    <li>
      <button
        type="button"
        className={`flow-pick flow-pick--${tone}`}
        onClick={() => onSelect(ticket.id)}
        title={`${ticket.title}\nShow on the canvas`}
      >
        <span className="flow-pick__id">#{ticket.id}</span>
        <span className="flow-pick__title">{ticket.title}</span>
        <span className="flow-pick__note">{note}</span>
      </button>
    </li>
  );
}
