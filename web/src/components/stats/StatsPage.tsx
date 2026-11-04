import type { ReactNode } from "react";
import type { StatsReport } from "../../api/types";
import { useStats } from "../../hooks/useStats";
import { navigate, STATS_HASH, type Route } from "../../lib/hashRoute";
import { periodFromParam, STATS_PERIODS, throughputDays } from "../../lib/stats";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { PageShell } from "../common/PageShell";
import { CompareTable } from "./CompareTable";
import { deliveryRows, mixRows, nowRows, qualityRows, stageTimeRows, wipRows } from "./statsRows";
import { ThroughputChart } from "./ThroughputChart";

/** Props for {@link StatsPage}. */
export interface StatsPageProps {
  route: Route;
}

/** Delivery stats (#/stats?days=N): autonomous tickets versus the owner's own. */
export function StatsPage({ route }: StatsPageProps) {
  const days = periodFromParam(route.params.get("days"));
  const { report, error, isLoading, refetch } = useStats(days);
  return (
    <PageShell page="stats" title="Stats" actions={<PeriodPicker days={days} />}>
      {report === null && isLoading && <LoadingState label="Loading stats" />}
      {report === null && !isLoading && error !== null && (
        <ErrorState message={`Couldn't load stats: ${error}`} onRetry={refetch} />
      )}
      {report !== null && error !== null && (
        <p className="banner banner--error" role="alert">
          Couldn't refresh stats: {error}{" "}
          <button type="button" className="link-button" onClick={refetch}>Retry</button>
        </p>
      )}
      {report !== null && <StatsReportView report={report} isRefreshing={isLoading} />}
    </PageShell>
  );
}

function PeriodPicker({ days }: { days: number }) {
  return (
    <fieldset className="segmented">
      <legend className="visually-hidden">Period</legend>
      {STATS_PERIODS.map((period) => (
        <label key={period} className={`segmented__option${period === days ? " segmented__option--active" : ""}`}>
          <input
            type="radio"
            name="stats-period"
            value={period}
            checked={period === days}
            onChange={() => navigate(`${STATS_HASH}?days=${period}`)}
            className="visually-hidden"
          />
          {period} days
        </label>
      ))}
    </fieldset>
  );
}

function StatsReportView({ report, isRefreshing }: { report: StatsReport; isRefreshing: boolean }) {
  const period = `last ${report.window_days} days`;
  return (
    <div className="stats" aria-busy={isRefreshing}>
      <p className="stats__intro">
        Autonomous tickets were filed from a chat you agreed and run to a pull request without stops; yours are
        tickets you created on the board. Periods end now and start at local midnight.
      </p>
      <StatsSection title={`Delivery, ${period}`}>
        <CompareTable caption="Delivery" rowHeader="Metric" rows={deliveryRows(report)} />
      </StatsSection>
      <StatsSection title="Merged per day">
        <ThroughputChart days={throughputDays(report)} />
      </StatsSection>
      <StatsSection title="Open work now">
        <CompareTable caption="Open work now" rowHeader="Metric" rows={nowRows(report)} />
        <CompareTable caption="Open tickets per stage" rowHeader="Stage" rows={wipRows(report)} />
      </StatsSection>
      <StatsSection title="Time in stage" note="Median · p90 (finished stays), for stays that ended in the period.">
        <CompareTable caption="Time in stage" rowHeader="Stage" rows={stageTimeRows(report)} />
      </StatsSection>
      <StatsSection title="Rework">
        <CompareTable caption="Rework" rowHeader="Metric" rows={qualityRows(report)} />
      </StatsSection>
      <StatsSection title="Agent mix" note="Models are as agents reported them; counts are tickets per model and claims per action or worker.">
        <CompareTable caption="Tickets per model" rowHeader="Model" rows={mixRows(report, (s) => s.models)} emptyText="No models reported in this period." />
        <CompareTable caption="Claims per action" rowHeader="Action" rows={mixRows(report, (s) => s.claims)} emptyText="No claims in this period." />
        <CompareTable caption="Claims per worker" rowHeader="Worker" rows={mixRows(report, (s) => s.agents)} emptyText="No claims in this period." />
      </StatsSection>
      <StatsSection title="By repository">
        <RepoTable report={report} />
      </StatsSection>
      <StatsSection title="Not recorded yet">
        <ul className="stats__gaps">
          <li>Windows verification pass or fail: the board only knows a build is waiting for Windows, not the result.</li>
          <li>Pull request review comments and approvals: only round trips from review back to build are recorded.</li>
          <li>When a pull request was opened on the provider: the time a ticket first reached review stands in for it.</li>
        </ul>
      </StatsSection>
    </div>
  );
}

function StatsSection({ title, note, children }: { title: string; note?: string; children: ReactNode }) {
  return (
    <section className="stats-section">
      <h2 className="stats-section__title">{title}</h2>
      {note !== undefined && <p className="stats-section__note">{note}</p>}
      {children}
    </section>
  );
}

function RepoTable({ report }: { report: StatsReport }) {
  if (report.repos.length === 0) {
    return <p className="stats-empty">No repository activity in this period.</p>;
  }
  return (
    <div className="stats-table__scroll">
      <table className="stats-table">
        <caption className="visually-hidden">Delivery by repository</caption>
        <thead>
          <tr>
            <th scope="col">Repository</th>
            <th scope="col" className="stats-table__num">Merged (auto · yours)</th>
            <th scope="col" className="stats-table__num">PRs opened (auto · yours)</th>
            <th scope="col" className="stats-table__num">Open (auto · yours)</th>
          </tr>
        </thead>
        <tbody>
          {report.repos.map((repo) => (
            <tr key={repo.repo_id}>
              <th scope="row">{repo.name}</th>
              <td className="stats-table__num">{repo.autonomous.merged} · {repo.owner.merged}</td>
              <td className="stats-table__num">{repo.autonomous.prs_opened} · {repo.owner.prs_opened}</td>
              <td className="stats-table__num">{repo.autonomous.open} · {repo.owner.open}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
