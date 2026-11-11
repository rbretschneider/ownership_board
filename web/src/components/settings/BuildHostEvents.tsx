import { useCallback } from "react";
import { useApi } from "../../api/ApiContext";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { eventDescription } from "../../lib/buildHosts";
import { relativeTime } from "../../lib/relativeTime";
import { ErrorState } from "../common/ErrorState";

/** The most recent events a build host reported, polled while shown. */
export function BuildHostEvents({ id }: { id: number }) {
  const api = useApi();
  const load = useCallback(() => api.buildHostEvents(id), [api, id]);
  const events = useVisiblePolling(load);
  return (
    <div className="build-host__events">
      <h4 className="field__label">Recent host events</h4>
      {events.error && <ErrorState message={events.error} onRetry={events.refresh} />}
      {events.value === null && !events.error && (
        <p role="status" className="muted small">
          Loading events…
        </p>
      )}
      {events.value?.length === 0 && <p className="muted small">No host events.</p>}
      {events.value !== null && events.value.length > 0 && (
        <ul className="build-host__event-list">
          {events.value.map((event) => (
            <li key={event.id}>
              <time dateTime={event.created_at} title={new Date(event.created_at).toLocaleString()}>
                {relativeTime(event.created_at)}
              </time>
              <span>{eventDescription(event)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
