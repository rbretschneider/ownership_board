import { useContext } from "react";
import { Monitor, Server, Laptop } from "lucide-react";
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
            const Icon = host.os === "windows" ? Monitor : host.os === "macos" ? Laptop : Server;
            return (
              <div key={host.id} className={`registered-runner${warning ? " registered-runner--warning" : ""}`}>
                <span className="registered-runner__identity">
                  <Icon size={14} aria-hidden="true" />
                  <strong title={host.name}>{host.name}</strong>
                  <span>{OS_LABELS[host.os]}</span>
                </span>
                <span className={`build-host-health ${host.online ? "build-host-health--online" : ""}`}>
                  {stale
                    ? host.online
                      ? "Last reported online"
                      : "Last reported offline"
                    : host.online
                      ? "Online"
                      : "Offline"}
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
                <span>
                  {OS_LABELS[host.os]} queue: {queued}
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
