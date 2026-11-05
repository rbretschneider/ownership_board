import { CircleAlert, Hammer, Hourglass, Server, TestTube } from "lucide-react";
import type { RunnerSlot } from "../../api/types";
import { useRunners } from "../../hooks/useRunners";
import { ticketHash } from "../../lib/hashRoute";
import { BLOCK_REASONS, formatMinutes, hostState, minutesSince, slotStep, slotTone } from "../../lib/runners";

const QUEUE_SHOWN = 6;

const HOST_LABELS = { unknown: "no report yet", offline: "not reporting", paused: "paused in Settings", online: "online" } as const;

function SlotChip({ slot, now }: { slot: RunnerSlot; now: number }) {
  const tone = slotTone(slot);
  const Icon = tone === "orphan" ? CircleAlert : tone === "waiting" ? Hourglass : tone === "testing" ? TestTube : Hammer;
  const elapsed = formatMinutes(minutesSince(slot.since, now));
  const ticket = slot.ticket_id === null ? `${slot.repo.split("/")[1]} ${slot.sha.slice(0, 7)}` : `#${slot.ticket_id}`;
  const step = tone === "orphan" ? "old commit · stopping" : slotStep(slot);
  const title = `Slot ${slot.slot}: ${slot.ticket_id === null ? "" : `#${slot.ticket_id} · `}${slot.repo} @ ${slot.sha} (${slot.preset}) · ${step}${elapsed ? ` · ${elapsed} on this job` : ""}${tone === "orphan" ? " · a newer commit of this ticket replaced it" : ""}`;
  const body = (
    <>
      <span className="runner__slot">{slot.slot}</span>
      <Icon size={12} aria-hidden="true" />
      <span className="runner__ticket">{ticket}</span>
      <span className="runner__step">{step}</span>
      {elapsed && <span className="runner__time">{elapsed}</span>}
    </>
  );
  return slot.ticket_id === null
    ? <span className={`runner runner--${tone}`} title={title}>{body}</span>
    : <a className={`runner runner--${tone}`} href={ticketHash(slot.ticket_id)} title={title}>{body}</a>;
}

/**
 * One line that answers "what is each Windows build slot doing right now":
 * the ticket, the step (building, testing, waiting for the desktop), how long,
 * plus what is next in line. Fed by the build host through the board.
 */
export function RunnersStrip() {
  const { snapshot, now } = useRunners();
  const state = hostState(snapshot, now);
  if (state === "unknown") return null;
  const slots = Math.max(snapshot.parallel_builds, ...snapshot.slots.map((s) => s.slot), 0);
  const seen = minutesSince(snapshot.observed_at, now);
  const hostTitle = `Windows build host ${snapshot.host || ""}: ${HOST_LABELS[state]}${state === "offline" ? `, last report ${formatMinutes(seen)} ago` : ""}`;
  const queued = snapshot.queue.slice(0, QUEUE_SHOWN);
  const block = state === "online" && snapshot.blocked !== "" ? BLOCK_REASONS[snapshot.blocked] : undefined;
  const blockedFor = formatMinutes(minutesSince(snapshot.blocked_since, now));
  return (
    <section className={`runners runners--${state}`} aria-label="Windows build slots">
      <span className="runners__host" title={hostTitle}>
        <Server size={13} aria-hidden="true" />
        <span className="runners__dot" aria-hidden="true" />
        Windows{state !== "online" && <span className="runners__state">{HOST_LABELS[state]}{state === "offline" && seen !== null ? ` · ${formatMinutes(seen)}` : ""}</span>}
      </span>
      {block && (
        <span className="runners__blocked" title={block.long} role="status">
          {block.short}{blockedFor ? ` · ${blockedFor}` : ""}
        </span>
      )}
      <span className="runners__slots">
        {Array.from({ length: slots }, (_, i) => i + 1).map((n) => {
          const slot = snapshot.slots.find((s) => s.slot === n);
          return slot
            ? <SlotChip key={n} slot={slot} now={now} />
            : <span key={n} className="runner runner--idle" title={`Slot ${n}: idle`}><span className="runner__slot">{n}</span><span className="runner__step">idle</span></span>;
        })}
      </span>
      {snapshot.queue.length > 0 && (
        <span className="runners__queue" title={snapshot.queue.map((q) => `${q.position}. ${q.ticket_id === null ? q.repo : `#${q.ticket_id}`} @ ${q.sha}${q.deferred_until ? " (retrying later)" : ""}`).join("\n")}>
          Next:
          {queued.map((q) => q.ticket_id === null
            ? <span key={q.position} className="runners__next">{q.sha.slice(0, 7)}</span>
            : <a key={q.position} className="runners__next" href={ticketHash(q.ticket_id)}>#{q.ticket_id}</a>)}
          {snapshot.queue.length > QUEUE_SHOWN && <span className="runners__more">+{snapshot.queue.length - QUEUE_SHOWN}</span>}
        </span>
      )}
    </section>
  );
}
