import { useEffect, useRef } from "react";
import type { Stage, TicketSummary } from "../api/types";
import {
  alertPreferences,
  alertText,
  canNotify,
  newlyWaiting,
  playDing,
  titleFor,
  unlockSound,
  waitingIds,
} from "../lib/ownerAlerts";
import { ticketHash } from "../lib/hashRoute";

/**
 * Keeps the tab title at "(n) ownership" and, when a ticket newly needs the
 * owner (a live update brought it in), plays a chime and, while the tab is in
 * the background, shows a system notification if the owner turned them on.
 * `tickets` is null until the board has loaded.
 */
export function useOwnerAlerts(tickets: TicketSummary[] | null, stages: Stage[]): void {
  const previous = useRef<Set<number> | null>(null);

  useEffect(() => {
    const unlock = () => unlockSound();
    window.addEventListener("pointerdown", unlock, { once: true });
    window.addEventListener("keydown", unlock, { once: true });
    return () => {
      window.removeEventListener("pointerdown", unlock);
      window.removeEventListener("keydown", unlock);
    };
  }, []);

  useEffect(() => {
    if (tickets === null) {
      return;
    }
    const waiting = waitingIds(tickets);
    document.title = titleFor(waiting.size);
    const fresh = newlyWaiting(previous.current, tickets);
    previous.current = waiting;
    if (fresh.length === 0) {
      return;
    }
    if (alertPreferences.isSoundOn()) {
      playDing();
    }
    if (alertPreferences.isNotifyOn() && canNotify() && Notification.permission === "granted" && document.hidden) {
      for (const ticket of fresh) {
        showNotification(ticket, stages);
      }
    }
  }, [tickets, stages]);

  useEffect(() => () => {
    document.title = titleFor(0);
  }, []);
}

function showNotification(ticket: TicketSummary, stages: Stage[]): void {
  const { title, body } = alertText(ticket, stages);
  const notification = new Notification(title, { body, tag: `ticket-${ticket.id}` });
  notification.onclick = () => {
    window.focus();
    window.location.hash = ticketHash(ticket.id);
    notification.close();
  };
}
