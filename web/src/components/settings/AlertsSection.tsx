import { useState } from "react";
import { alertPreferences, canNotify, playDing, unlockSound } from "../../lib/ownerAlerts";

/**
 * How this browser tells the owner a ticket needs them: the tab title always
 * shows the count; a chime and, on HTTPS, system notifications are optional.
 */
export function AlertsSection() {
  const [isSoundOn, setSoundOn] = useState(alertPreferences.isSoundOn);
  const [isNotifyOn, setNotifyOn] = useState(() => alertPreferences.isNotifyOn() && canNotify() && Notification.permission === "granted");
  const [notice, setNotice] = useState<string | null>(null);
  const isNotifyAvailable = canNotify();

  const changeSound = (next: boolean) => {
    alertPreferences.setSoundOn(next);
    setSoundOn(next);
    if (next) {
      unlockSound();
      playDing();
    }
  };

  const changeNotify = async (next: boolean) => {
    setNotice(null);
    if (!next) {
      alertPreferences.setNotifyOn(false);
      setNotifyOn(false);
      return;
    }
    const permission = Notification.permission === "default" ? await Notification.requestPermission() : Notification.permission;
    if (permission !== "granted") {
      setNotice("The browser blocked notifications for this site. Allow them in the site settings, then try again.");
      return;
    }
    alertPreferences.setNotifyOn(true);
    setNotifyOn(true);
  };

  return (
    <section className="settings-section" aria-labelledby="alerts-title">
      <h3 id="alerts-title" className="settings-section__title">Alerts</h3>
      <p className="muted small">The tab title shows how many tickets wait for you, for example "(2) ownership".</p>
      <label className="switch" htmlFor="alert-sound">
        <input
          id="alert-sound"
          type="checkbox"
          role="switch"
          className="switch__input"
          checked={isSoundOn}
          onChange={(e) => changeSound(e.target.checked)}
        />
        <span className="switch__track" aria-hidden="true" />
        <span className="switch__label">Chime when a ticket needs you</span>
      </label>
      <label className="switch" htmlFor="alert-notify">
        <input
          id="alert-notify"
          type="checkbox"
          role="switch"
          className="switch__input"
          checked={isNotifyOn}
          disabled={!isNotifyAvailable}
          onChange={(e) => void changeNotify(e.target.checked)}
        />
        <span className="switch__track" aria-hidden="true" />
        <span className="switch__label">Desktop notification while the tab is in the background</span>
      </label>
      {!isNotifyAvailable && (
        <p className="muted small">
          Browsers only allow notifications on HTTPS. This board is served over plain HTTP, so use the chime
          and the title count, or ntfy on your phone.
        </p>
      )}
      {notice !== null && <p className="form-error" role="alert">{notice}</p>}
    </section>
  );
}
