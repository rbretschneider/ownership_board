import type { Stage, TicketSummary } from "../api/types";
import { preferences, STORAGE_KEYS } from "./storage";

const APP_TITLE = "ownership";

/** The tab title: "(3) ownership" while three tickets wait for the owner. */
export function titleFor(waiting: number): string {
  return waiting > 0 ? `(${waiting}) ${APP_TITLE}` : APP_TITLE;
}

/**
 * Tickets that wait for the owner now but did not before. `previous` is null on
 * the first load, which never alerts: those tickets were already waiting.
 */
export function newlyWaiting(previous: ReadonlySet<number> | null, tickets: TicketSummary[]): TicketSummary[] {
  if (previous === null) {
    return [];
  }
  return tickets.filter((t) => t.needs_owner && !previous.has(t.id));
}

/** The ids of tickets waiting for the owner. */
export function waitingIds(tickets: TicketSummary[]): Set<number> {
  return new Set(tickets.filter((t) => t.needs_owner).map((t) => t.id));
}

/**
 * Notification text. Fixed board words only (number and stage), never the
 * title or anything an agent wrote, as with ntfy pushes.
 */
export function alertText(ticket: TicketSummary, stages: Stage[]): { title: string; body: string } {
  const stage = stages.find((s) => s.id === ticket.stage)?.label ?? ticket.stage;
  let reason = stage;
  if (ticket.blocked_reason !== "") {
    reason = "Blocked";
  } else if (ticket.open_questions > 0) {
    reason = ticket.open_questions === 1 ? "1 question" : `${ticket.open_questions} questions`;
  }
  return { title: `#${ticket.id} needs you`, body: reason };
}

/** Whether this page may show system notifications at all (HTTPS or localhost). */
export function canNotify(): boolean {
  return typeof window !== "undefined" && window.isSecureContext && "Notification" in window;
}

/** Alert preferences, stored per browser. Sound is on by default, notifications off. */
export const alertPreferences = {
  isSoundOn: () => preferences.read(STORAGE_KEYS.alertSound, "on") === "on",
  setSoundOn: (isOn: boolean) => preferences.write(STORAGE_KEYS.alertSound, isOn ? "on" : "off"),
  isNotifyOn: () => preferences.read(STORAGE_KEYS.alertNotify, "off") === "on",
  setNotifyOn: (isOn: boolean) => preferences.write(STORAGE_KEYS.alertNotify, isOn ? "on" : "off"),
};

let audio: AudioContext | null = null;

/**
 * Prepares sound playback. Browsers only allow audio after the person has
 * interacted with the page, so this runs on the first click or key press.
 */
export function unlockSound(): void {
  try {
    audio ??= new AudioContext();
    void audio.resume();
  } catch {
    // No Web Audio: the title count still works.
  }
}

/** A short two-note chime, generated so there is no sound file to load. */
export function playDing(): void {
  if (audio === null || audio.state !== "running") {
    return;
  }
  const start = audio.currentTime;
  for (const [index, frequency] of [880, 1320].entries()) {
    const at = start + index * 0.12;
    const tone = audio.createOscillator();
    const gain = audio.createGain();
    tone.type = "sine";
    tone.frequency.value = frequency;
    gain.gain.setValueAtTime(0.0001, at);
    gain.gain.exponentialRampToValueAtTime(0.18, at + 0.01);
    gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.25);
    tone.connect(gain).connect(audio.destination);
    tone.start(at);
    tone.stop(at + 0.3);
  }
}
