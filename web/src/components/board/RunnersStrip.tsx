import { useContext } from "react";
import { ListOrdered } from "lucide-react";
import { PlatformGlyph } from "../common/PlatformIcon";
import { RegisteredRunnersContext } from "../../hooks/useRunners";
import { OS_LABELS } from "../../lib/buildHosts";
import { ticketHash } from "../../lib/hashRoute";
import { LegacyRunnersStrip } from "./LegacyRunnersStrip";

export function RunnersStrip() {
  const { snapshot, stale, error, refresh } = useContext(RegisteredRunnersContext);
  return (
    <>
      {(snapshot || error) && (
        <section
          className={`registered-runners${stale ? " registered-runners--stale" : ""}`}
          aria-label="Registered build hosts"
          tabIndex={0}
        >
          {error && (
            <p role="alert">
              Registered host update failed: {error}{" "}
              <button type="button" onClick={refresh}>
                Retry host status
              </button>
            </p>
          )}
          {stale && <p role="status">Status reading is stale — connectivity below is the last reported state.</p>}
          {snapshot?.hosts.map((host) => {
            const queued = snapshot.queued_by_os[host.os] ?? 0;
            const warning = !stale && !host.online && queued > 0;
            const health = stale ? (host.online ? "Last reported online" : "Last reported offline") : host.online ? "Online" : "Offline";
            // Status colors: the dot is connectivity; the platform icon is what the host is doing.
            const work = !host.online ? "failed" : host.jobs.length > 0 ? "running" : "passed";
            const workWords = !host.online ? "offline" : host.jobs.length > 0 ? `building ${host.jobs.length} of ${host.max_parallel}` : "idle";
            const queueLabel = `${OS_LABELS[host.os]} queue: ${queued}`;
            return (
              <div key={host.id} className={`registered-runner${warning ? " registered-runner--warning" : ""}`}>
                <span className="registered-runner__identity">
                  <span className={`host-dot host-dot--${stale ? "stale" : host.online ? "online" : "offline"}`} role="img" title={health} aria-label={health} />
                  <strong title={host.name}>{host.name}</strong>
                  <span className={`platform platform--${work}`} role="img" data-platform={host.os}
                    title={`${OS_LABELS[host.os]} build host · ${workWords}`} aria-label={`${OS_LABELS[host.os]} build host · ${workWords}`}>
                    <PlatformGlyph platform={host.os} />
                  </span>
                </span>
                {!host.enabled && <span>Disabled</span>}
                {!host.last_seen_at && <span>never seen</span>}
                <span className="runners__slots">
                  {host.jobs.map((job) => {
                    const stopping = job.status === "cancelled" || job.status === "superseded";
                    const label = stopping ? "stopping" : job.detail || job.phase || job.status;
                    return (
                      <a
                        key={job.id}
                        className={`runner${stopping ? " runner--orphan" : ""}`}
                        href={ticketHash(job.ticket_id)}
                        title={`${job.repo} @ ${job.sha} (${job.preset}) · ${label} · attempt ${job.attempt}`}
                      >
                        <span>#{job.ticket_id}</span>
                        <span className="runner__step">{label}</span>
                      </a>
                    );
                  })}
                  {Array.from({ length: Math.max(0, host.max_parallel - host.jobs.length) }, (_, i) => (
                    <span key={`idle-${i}`} className="runner runner--idle">
                      idle
                    </span>
                  ))}
                </span>
                <span className="registered-runner__queue" title={queueLabel} aria-label={queueLabel}>
                  <ListOrdered size={13} aria-hidden="true" />
                  {queued}
                </span>
                {warning && <strong>Offline with queued work</strong>}
              </div>
            );
          })}
        </section>
      )}
      <LegacyRunnersStrip />
    </>
  );
}
