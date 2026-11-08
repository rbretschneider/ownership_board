import type { RepoHealth, StatsRepo } from "../../api/types";
import { ticketHash } from "../../lib/hashRoute";

/** Current review cadence and the outstanding work from each latest review. */
export function HealthTable({ repos }: { repos: StatsRepo[] }) {
  const rows = repos.filter((repo): repo is StatsRepo & { health: RepoHealth } => repo.health != null);
  if (rows.length === 0) {
    return <p className="stats-empty">No repositories configured.</p>;
  }
  return (
    <div className="stats-table__scroll">
      <table className="stats-table">
        <caption className="visually-hidden">Architecture health</caption>
        <thead>
          <tr>
            <th scope="col">Repository</th>
            <th scope="col">Last check</th>
            <th scope="col" className="stats-table__num">Merges until next</th>
            <th scope="col" className="stats-table__num">Open follow-ups</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(({ repo_id, name, health }) => (
            <tr key={repo_id}>
              <th scope="row">{name}</th>
              <td>
                {health.last_check_ticket !== null && <a href={ticketHash(health.last_check_ticket)}>#{health.last_check_ticket}</a>}
                {health.last_check_ticket !== null && health.last_checked_at !== null && " · "}
                {health.last_checked_at !== null && (
                  <time dateTime={health.last_checked_at}>{new Date(health.last_checked_at).toLocaleString()}</time>
                )}
                {health.last_check_ticket === null && health.last_checked_at === null && "Never"}
              </td>
              <td className="stats-table__num">{health.every === 0 ? "off" : health.next_in}</td>
              <td className="stats-table__num">{health.open_follow_ups}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
