import type { ReactNode } from "react";
import { SEGMENTS, type SegmentKey } from "../../lib/stats";

/** One metric, shown for both segments. */
export interface CompareRow {
  label: string;
  /** Explains how the metric is measured. */
  hint?: string;
  values: Record<SegmentKey, ReactNode>;
}

/** Props for {@link CompareTable}. */
export interface CompareTableProps {
  caption: string;
  /** Header of the label column. */
  rowHeader: string;
  rows: CompareRow[];
  /** Shown instead of an empty table. */
  emptyText?: string;
}

/** A metric table with one column per segment: autonomous tickets versus the owner's. */
export function CompareTable({ caption, rowHeader, rows, emptyText }: CompareTableProps) {
  if (rows.length === 0) {
    return <p className="stats-empty">{emptyText ?? "Nothing recorded in this period."}</p>;
  }
  return (
    <div className="stats-table__scroll">
      <table className="stats-table">
        <caption className="visually-hidden">{caption}</caption>
        <thead>
          <tr>
            <th scope="col">{rowHeader}</th>
            {SEGMENTS.map((s) => <th key={s.key} scope="col" className="stats-table__num">{s.label}</th>)}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.label}>
              <th scope="row">
                {row.label}
                {row.hint !== undefined && <span className="stats-table__hint">{row.hint}</span>}
              </th>
              {SEGMENTS.map((s) => <td key={s.key} className="stats-table__num">{row.values[s.key]}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
