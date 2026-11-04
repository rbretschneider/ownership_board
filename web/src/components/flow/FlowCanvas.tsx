import { memo, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import {
  FLOW_SIZES,
  nodeStatus,
  pathEdgeKeys,
  retries,
  RETRY_HOT,
  RETRY_WARM,
  type FlowFocus,
  type FlowLayout,
  type FlowNode,
} from "../../lib/flow";

/** Props for {@link FlowCanvas}. */
export interface FlowCanvasProps {
  layout: FlowLayout;
  focus: FlowFocus | null;
  showCritical: boolean;
  now: number;
  /** Toggles the focus on a ticket. */
  onSelect: (id: number) => void;
}

const S = FLOW_SIZES;

/**
 * Swimlanes of tickets drifting right towards merged, with soft lines from
 * each ticket to the work it waits on. Scrolls sideways inside the page and
 * opens at the right-hand end, where work is moving.
 */
export function FlowCanvas({ layout, focus, showCritical, now, onSelect }: FlowCanvasProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const hasScrolled = useRef(false);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (el !== null && !hasScrolled.current) {
      el.scrollLeft = el.scrollWidth;
      hasScrolled.current = true;
    }
  }, [layout]);

  // Bring a ticket picked from the cards (or anywhere) into view.
  useEffect(() => {
    if (focus === null) return;
    const el = scrollRef.current?.querySelector<HTMLElement>(`[data-ticket="${focus.id}"]`);
    el?.scrollIntoView?.({ block: "nearest", inline: "center", behavior: "smooth" });
  }, [focus?.id]);

  const criticalKeys = useMemo(() => {
    if (focus !== null) return pathEdgeKeys(focus.path);
    if (!showCritical) return new Set<string>();
    const keys = new Set<string>();
    for (const lane of layout.lanes) for (const key of pathEdgeKeys(lane.criticalPath)) keys.add(key);
    return keys;
  }, [focus, layout, showCritical]);

  const criticalNodes = useMemo(() => {
    const ids = new Set<number>();
    if (focus !== null) focus.path.forEach((id) => ids.add(id));
    else if (showCritical) layout.lanes.forEach((lane) => lane.criticalPath.forEach((id) => ids.add(id)));
    return ids;
  }, [focus, layout, showCritical]);

  const isLit = (id: number) => focus === null || id === focus.id || focus.upstream.has(id) || focus.downstream.has(id);

  return (
    <div className="flow-canvas" ref={scrollRef} tabIndex={-1}>
      <div className="flow-canvas__inner" style={{ width: layout.width, height: layout.height }}>
        {layout.columns.map((column) => (
          <div
            key={column.key}
            className={`flow-col${column.isCollapsed ? " flow-col--collapsed" : ""}${column.depth !== null ? " flow-col--queue" : ""}`}
            style={{ left: column.x, width: column.width, height: layout.height }}
            title={`${column.label}: ${column.about}`}
          >
            <span className="flow-col__label">{column.label}</span>
            {!column.isCollapsed && <span className="flow-col__count">{column.count}</span>}
          </div>
        ))}
        {layout.lanes.map((lane) => {
          const milestone = lane.milestoneId === null ? undefined : layout.graph.byId.get(lane.milestoneId);
          return (
            <section
              key={lane.key}
              className="flow-lane"
              style={{ top: lane.y, height: lane.height, width: layout.width }}
              aria-label={`${lane.name} swimlane`}
            >
              <header className="flow-lane__head">
                <span className="flow-lane__dot" style={lane.color === "" ? undefined : { background: lane.color }} aria-hidden="true" />
                <strong className="flow-lane__name">{lane.name}</strong>
                <span className="flow-lane__facts">
                  {lane.open} open · {lane.inFlight} in flight · {lane.queued} queued · {lane.merged} merged this week
                  {milestone !== undefined && lane.criticalPath.length > 1 && (
                    <> · longest chain {lane.criticalPath.length - 1} to #{milestone.id}</>
                  )}
                </span>
              </header>
            </section>
          );
        })}
        <svg className="flow-edges" width={layout.width} height={layout.height} aria-hidden="true">
          {layout.edges.map(({ from, to }) => {
            const a = layout.nodes.get(from) as FlowNode;
            const b = layout.nodes.get(to) as FlowNode;
            const key = `${from}>${to}`;
            const isCritical = criticalKeys.has(key);
            const lit = focus === null ? true : isLit(from) && isLit(to) && (from === focus.id || to === focus.id || focus.upstream.has(from) || focus.downstream.has(to));
            const tone = nodeStatus(b.ticket, now).tone;
            return (
              <path
                key={key}
                className={`flow-edge flow-edge--${tone}${isCritical ? " flow-edge--critical" : ""}${lit ? "" : " flow-edge--dim"}`}
                d={edgePath(a, b)}
              />
            );
          })}
        </svg>
        {[...layout.nodes.values()].map((node) => (
          <FlowNodeButton
            key={node.ticket.id}
            node={node}
            gates={layout.gates.get(node.ticket.id) ?? 0}
            now={now}
            isSelected={focus?.id === node.ticket.id}
            isDimmed={!isLit(node.ticket.id)}
            isCritical={criticalNodes.has(node.ticket.id)}
            onSelect={onSelect}
          />
        ))}
      </div>
    </div>
  );
}

/** A soft curve from the right edge of the waiting ticket to the left edge of what it waits on. */
function edgePath(from: FlowNode, to: FlowNode): string {
  const x1 = from.x + S.nodeWidth;
  const y1 = from.y + S.nodeHeight / 2;
  const x2 = to.x;
  const y2 = to.y + S.nodeHeight / 2;
  const dx = Math.max(24, (x2 - x1) / 2);
  return `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`;
}

interface FlowNodeButtonProps {
  node: FlowNode;
  gates: number;
  now: number;
  isSelected: boolean;
  isDimmed: boolean;
  isCritical: boolean;
  onSelect: (id: number) => void;
}

const FlowNodeButton = memo(function FlowNodeButton({ node, gates, now, isSelected, isDimmed, isCritical, onSelect }: FlowNodeButtonProps) {
  const t = node.ticket;
  const status = nodeStatus(t, now);
  const loops = retries(t);
  const heat = loops.total >= RETRY_HOT ? "hot" : loops.total >= RETRY_WARM ? "warm" : "";
  const tooltip = [
    `#${t.id} ${t.title}`,
    status.label,
    gates > 0 && `Gates ${gates} ticket${gates === 1 ? "" : "s"}`,
    t.waiting_on.length > 0 && `Waits on ${t.waiting_on.map((id) => `#${id}`).join(", ")}`,
    loops.detail,
    t.blocked_reason,
  ].filter(Boolean).join("\n");
  return (
    <button
      type="button"
      data-ticket={t.id}
      className={`flow-node flow-node--${status.tone}${isSelected ? " flow-node--selected" : ""}${isDimmed ? " flow-node--dim" : ""}${isCritical ? " flow-node--critical" : ""}`}
      style={{ left: node.x, top: node.y, width: S.nodeWidth, height: S.nodeHeight }}
      onClick={() => onSelect(t.id)}
      aria-pressed={isSelected}
      title={tooltip}
    >
      <span className="flow-node__top">
        <span className="flow-node__id">#{t.id}</span>
        {gates > 0 && <span className="flow-node__badge flow-node__badge--gates">gates {gates}</span>}
        {loops.total > 0 && (
          <span className={`flow-node__badge flow-node__badge--retry${heat === "" ? "" : ` flow-node__badge--${heat}`}`}>↻{loops.total}</span>
        )}
      </span>
      <span className="flow-node__title">{t.title}</span>
      <span className="flow-node__status">
        <span className={`flow-dot flow-dot--${status.tone}`} aria-hidden="true" />
        <span className="flow-node__status-label">{status.label}</span>
      </span>
    </button>
  );
});
