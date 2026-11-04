import { useCallback, useEffect, useMemo, useState } from "react";
import type { RepoFilter } from "../../api/boardApi";
import { useBoard } from "../../hooks/useBoard";
import { useDebouncedCallback } from "../../hooks/useDebouncedCallback";
import { useEventStream } from "../../hooks/useEventStream";
import { useStats } from "../../hooks/useStats";
import { REFETCH_DEBOUNCE_MS } from "../../lib/constants";
import { flowInsights, focusOn, forecastLane, laneOptions, layoutFlow } from "../../lib/flow";
import { preferences, STORAGE_KEYS } from "../../lib/storage";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { PageShell } from "../common/PageShell";
import { FlowCanvas } from "./FlowCanvas";
import { FlowFilters } from "./FlowFilters";
import { FlowFocusBar } from "./FlowFocusBar";
import { FlowInsightCards } from "./FlowInsightCards";

const ALL_REPOS: RepoFilter = { kind: "all" };
const STATS_DAYS = 30;

function readHidden(): Set<string> {
  return new Set(preferences.read(STORAGE_KEYS.flowHidden, "").split(",").filter((k) => k !== ""));
}

/**
 * The product owner's flow view (#/flow): read-only, from GET /board and
 * GET /stats. Every repo on one canvas, filterable: where work is stuck, what
 * the owner can unblock, whether it is moving, where agents loop, and a rough
 * finish estimate per repo.
 */
export function FlowPage() {
  const { board, error, isLoading, refetch } = useBoard(ALL_REPOS);
  const { report } = useStats(STATS_DAYS);
  const debouncedRefetch = useDebouncedCallback(refetch, REFETCH_DEBOUNCE_MS);
  useEventStream(debouncedRefetch, refetch);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [showCritical, setShowCritical] = useState(true);
  const [isDetailed, setDetailed] = useState(() => preferences.read(STORAGE_KEYS.flowDetailed, "") === "1");
  const [hidden, setHidden] = useState<Set<string>>(readHidden);

  // "Now" moves with each refresh, so merged-in-24h and live leases stay honest.
  const now = useMemo(() => Date.now(), [board]);
  const options = useMemo(() => (board === null ? [] : laneOptions(board.tickets, board.repos)), [board]);
  const laneKeys = useMemo(() => new Set(options.map((o) => o.key).filter((k) => !hidden.has(k))), [options, hidden]);
  const layout = useMemo(
    () => (board === null ? null : layoutFlow(board.tickets, board.repos, now, { isDetailed, laneKeys })),
    [board, now, isDetailed, laneKeys],
  );
  const insights = useMemo(() => {
    if (board === null || layout === null) return null;
    const shown = board.tickets.filter((t) => laneKeys.has(t.repo_id === null ? "none" : String(t.repo_id)));
    return flowInsights(shown, layout.gates, now);
  }, [board, layout, laneKeys, now]);
  const cycleHours = report !== null && report.all.cycle_time.count > 0 ? report.all.cycle_time.median_hours : null;
  const forecasts = useMemo(
    () => new Map((layout?.lanes ?? []).map((lane) => [lane.key, forecastLane(lane, cycleHours)])),
    [layout, cycleHours],
  );
  // A selected ticket that left the canvas (merged long ago, filtered out) clears the focus.
  const focus = useMemo(
    () => (layout === null || selectedId === null || !layout.nodes.has(selectedId) ? null : focusOn(layout, selectedId)),
    [layout, selectedId],
  );

  const select = useCallback((id: number | null) => {
    setSelectedId((current) => (id === current ? null : id));
  }, []);

  const toggleLane = useCallback((key: string) => {
    setHidden((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      preferences.write(STORAGE_KEYS.flowHidden, [...next].join(","));
      return next;
    });
  }, []);

  const showOnly = useCallback((key: string | null) => {
    const next = new Set(key === null ? [] : options.map((o) => o.key).filter((k) => k !== key));
    preferences.write(STORAGE_KEYS.flowHidden, [...next].join(","));
    setHidden(next);
  }, [options]);

  const changeDetailed = useCallback((value: boolean) => {
    preferences.write(STORAGE_KEYS.flowDetailed, value ? "1" : "");
    setDetailed(value);
  }, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSelectedId(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <PageShell page="flow" title="Flow" isWide>
      {board === null && isLoading && <LoadingState label="Loading flow" />}
      {board === null && !isLoading && error !== null && (
        <ErrorState message={`Couldn't load the board: ${error}`} onRetry={refetch} />
      )}
      {board !== null && error !== null && (
        <p className="banner banner--error" role="alert">
          Couldn't refresh: {error}{" "}
          <button type="button" className="link-button" onClick={refetch}>Retry</button>
        </p>
      )}
      {layout !== null && insights !== null && (
        <div className="flow" aria-busy={isLoading}>
          <FlowFilters
            options={options}
            hidden={hidden}
            onToggle={toggleLane}
            onShowOnly={showOnly}
            isDetailed={isDetailed}
            onDetailedChange={changeDetailed}
            showCritical={showCritical}
            onShowCriticalChange={setShowCritical}
          />
          <FlowInsightCards insights={insights} lanes={layout.lanes} forecasts={forecasts} cycleHours={cycleHours} now={now} onSelect={(id) => setSelectedId(id)} />
          <FlowFocusBar layout={layout} focus={focus} now={now} onClear={() => setSelectedId(null)} />
          {layout.lanes.length === 0
            ? <p className="flow__empty">{options.length === 0 ? "No open tickets. Everything has merged." : "Every repo is filtered out."}</p>
            : <FlowCanvas layout={layout} focus={focus} showCritical={showCritical} now={now} onSelect={select} />}
        </div>
      )}
    </PageShell>
  );
}
