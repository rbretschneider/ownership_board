import { useCallback, useEffect, useRef, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BuildHost, BuildHostEdit } from "../../api/types";
import { eventDescription, OS_LABELS } from "../../lib/buildHosts";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { ErrorState } from "../common/ErrorState";
import { BuildHostCapabilities } from "./BuildHostCapabilities";
import { BuildHostFacts } from "./BuildHostFacts";

type Props = {
  host: BuildHost;
  onMutate: (id: number, operation: () => Promise<BuildHost>) => Promise<void>;
  onRevoke: () => void;
};
function HostEvents({ id }: { id: number }) {
  const api = useApi();
  const load = useCallback(() => api.buildHostEvents(id), [api, id]);
  const events = useVisiblePolling(load);
  return (
    <div>
      <h4>Recent host events</h4>
      {events.error && <ErrorState message={events.error} onRetry={events.refresh} />}
      {events.value === null && !events.error && <p role="status">Loading events…</p>}
      {events.value?.length === 0 && <p>No host events.</p>}
      <ul>
        {events.value?.map((event) => (
          <li key={event.id}>
            <time dateTime={event.created_at}>{new Date(event.created_at).toLocaleString()}</time> ·{" "}
            {eventDescription(event)}
          </li>
        ))}
      </ul>
    </div>
  );
}
export function BuildHostRow({ host, onMutate, onRevoke }: Props) {
  const api = useApi();
  const [capabilities, setCapabilities] = useState(host.capabilities),
    [capacity, setCapacity] = useState(String(host.max_parallel));
  const dirty = useRef({ capabilities: false, capacity: false });
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null),
    [expanded, setExpanded] = useState(false);
  useEffect(() => {
    if (!dirty.current.capabilities) setCapabilities(host.capabilities);
    if (!dirty.current.capacity) setCapacity(String(host.max_parallel));
  }, [host]);
  const mutate = async (operation: () => Promise<BuildHost>, clearDraft = false) => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await onMutate(host.id, async () => {
        const next = await operation();
        if (clearDraft) {
          dirty.current = { capabilities: false, capacity: false };
          setCapabilities(next.capabilities);
          setCapacity(String(next.max_parallel));
        }
        return next;
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };
  const n = Number(capacity),
    valid = Number.isInteger(n) && n >= 1 && n <= 8;
  const pending = host.status === "pending",
    revoked = host.status === "revoked";
  const save = () => {
    const edit: BuildHostEdit = {};
    if (dirty.current.capacity) edit.max_parallel = n;
    if (dirty.current.capabilities) edit.capabilities = capabilities;
    void mutate(() => api.editBuildHost(host.id, edit), true);
  };
  return (
    <article className="build-host-row" aria-label={host.name}>
      <div className="build-host-row__heading">
        <strong>{host.name}</strong>
        <span>{OS_LABELS[host.os]}</span>
        {!pending && !revoked && (
          <>
            <span className={`build-host-health ${host.online ? "build-host-health--online" : ""}`}>
              {host.online ? "Online" : "Offline"}
            </span>
            <span>Last seen: {host.last_seen_at ? new Date(host.last_seen_at).toLocaleString() : "never seen"}</span>
          </>
        )}
        <span>Agent: {host.agent_version || "Unknown"}</span>
      </div>
      {revoked ? (
        <p>Revoked {host.revoked_at ? new Date(host.revoked_at).toLocaleString() : ""}</p>
      ) : (
        <>
          {pending ? (
            <BuildHostFacts host={host} />
          ) : (
            <label>
              <input
                type="checkbox"
                role="switch"
                aria-label={`Enable ${host.name}`}
                checked={host.enabled}
                disabled={busy}
                onChange={(e) => void mutate(() => api.editBuildHost(host.id, { enabled: e.target.checked }))}
              />{" "}
              Enabled
            </label>
          )}
          <fieldset disabled={busy}>
            <legend>{pending ? "Approval capabilities" : "Host configuration"}</legend>
            {!pending && (
              <label>
                Max parallel{" "}
                <input
                  className="input"
                  type="number"
                  min="1"
                  max="8"
                  step="1"
                  value={capacity}
                  onChange={(e) => {
                    dirty.current.capacity = true;
                    setCapacity(e.target.value);
                  }}
                />
              </label>
            )}
            {!pending && !valid && <p role="alert">Max parallel must be an integer from 1 through 8.</p>}
            <BuildHostCapabilities
              values={capabilities}
              onChange={(values) => {
                dirty.current.capabilities = true;
                setCapabilities(values);
              }}
            />
            {pending ? (
              <button
                type="button"
                className="button"
                onClick={() => void mutate(() => api.approveBuildHost(host.id, { capabilities }), true)}
              >
                Approve
              </button>
            ) : (
              <button
                type="button"
                className="button"
                disabled={!valid || (!dirty.current.capacity && !dirty.current.capabilities)}
                onClick={save}
              >
                Save
              </button>
            )}
            <button type="button" className="button button--danger" onClick={onRevoke}>
              {pending ? "Reject" : "Revoke"}
            </button>
          </fieldset>
          {error && <p role="alert">{error}</p>}
          {!pending && (
            <>
              <button type="button" className="button" aria-expanded={expanded} onClick={() => setExpanded((v) => !v)}>
                {expanded ? "Hide details" : "Show details"}
              </button>
              {expanded && (
                <>
                  <BuildHostFacts host={host} />
                  <HostEvents id={host.id} />
                </>
              )}
            </>
          )}
        </>
      )}
    </article>
  );
}
