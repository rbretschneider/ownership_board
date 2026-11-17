import { useEffect, useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BoardSettings, ManagerFlag, ManagerSession, ManagerStatus } from "../../api/types";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { useToast } from "../toast/ToastContext";

type ManagerSettings = Pick<BoardSettings,
  "manager_enabled" | "manager_build_cap" | "manager_stall_minutes" | "manager_daily_budget_usd" | "manager_budget_off">;

const FLAG_LABELS: Record<ManagerFlag["kind"], string> = {
  escalated: "Escalated",
  cycling: "Cycling",
  build_cap: "Over build cap",
  stalled: "Stalled",
};

/**
 * The board manager: an agent that keeps autonomous tickets moving at the
 * lowest token cost. It answers their questions, clears blockers and stops
 * cycles. Its decisions appear on each ticket's timeline.
 */
export function ManagerSection() {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [settings, setSettings] = useState<ManagerSettings | null>(null);
  const [status, setStatus] = useState<ManagerStatus | null>(null);
  const [flags, setFlags] = useState<ManagerFlag[]>([]);
  const [sessions, setSessions] = useState<ManagerSession[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    Promise.all([api.settings(), api.managerStatus(), api.managerFlags(), api.managerSessions()]).then(
      ([s, st, f, ses]) => {
        if (!active) return;
        setSettings(pick(s));
        setStatus(st);
        setFlags(f.flags);
        setSessions(ses.sessions);
      },
      (cause) => { if (active) setError(String(cause)); },
    );
    return () => { active = false; };
  }, [api]);

  const save = async (patch: Partial<ManagerSettings>) => {
    if (!settings) return;
    const before = settings;
    setSettings({ ...settings, ...patch });
    setBusy(true);
    try {
      setSettings(pick(await api.editSettings(patch)));
    } catch (cause) {
      setSettings(before);
      toast.error(`Couldn't save manager settings: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setBusy(false);
    }
  };

  const numberField = (key: keyof Omit<ManagerSettings, "manager_enabled" | "manager_budget_off">, label: string, hint: string, min: number, max: number) =>
    settings && (
      <div className="field">
        <label className="field__label" htmlFor={`${id}-${key}`}>{label}</label>
        <input className="input" id={`${id}-${key}`} type="number" min={min} max={max} defaultValue={settings[key]}
          disabled={busy} aria-describedby={`${id}-${key}-hint`}
          onBlur={(e) => {
            const value = Number(e.target.value);
            if (Number.isInteger(value) && value !== settings[key]) void save({ [key]: value } as Partial<ManagerSettings>);
          }} />
        <p className="field__hint" id={`${id}-${key}-hint`}>{hint}</p>
      </div>
    );

  return (
    <section className="settings-section" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="settings-section__title">Board manager</h2>
      {error && <ErrorState message={error} onRetry={() => window.location.reload()} />}
      {!settings && !error && <LoadingState label="Loading manager settings" />}
      {settings && (
        <>
          <div className="field">
            <label className="switch" htmlFor={`${id}-enabled`}>
              <input id={`${id}-enabled`} type="checkbox" role="switch" className="switch__input" checked={settings.manager_enabled}
                disabled={busy} onChange={(e) => void save({ manager_enabled: e.target.checked })} />
              <span className="switch__track" aria-hidden="true" />
              <span className="switch__label">Manager on</span>
            </label>
            <p className="muted small">
              Watches autonomous tickets only. It answers their questions, unblocks or parks them, leaves directions for the
              next pass and files tightly scoped repair tickets. Product-level answers are logged on the ticket so you can
              override them. Run it with a token created as Board manager.
            </p>
          </div>
          {numberField("manager_build_cap", "Build cap", "Flag a ticket once it has more build passes than this.", 1, 50)}
          {numberField("manager_stall_minutes", "Stall after (minutes)", "Flag a ticket nobody has touched for this long.", 15, 2880)}
          <div className="field">
            <label className="switch" htmlFor={`${id}-budget-off`}>
              <input id={`${id}-budget-off`} type="checkbox" role="switch" className="switch__input" checked={!settings.manager_budget_off}
                disabled={busy} onChange={(e) => void save({ manager_budget_off: !e.target.checked })} />
              <span className="switch__track" aria-hidden="true" />
              <span className="switch__label">Daily budget</span>
            </label>
            <p className="muted small">Off: the manager always runs its sessions, with no daily cap. Spend is still counted below.</p>
          </div>
          {!settings.manager_budget_off &&
            numberField("manager_daily_budget_usd", "Daily budget (USD)", "Model spend allowed per UTC day. 0 means flags only, no model sessions.", 0, 500)}
          {status && (
            <p className="muted small">
              {settings.manager_budget_off
                ? `Today: $${status.today.cost_usd.toFixed(2)} in ${status.today.sessions} session(s); no daily cap.`
                : `Today: $${status.today.cost_usd.toFixed(2)} in ${status.today.sessions} session(s); $${Math.max(0, status.remaining_usd).toFixed(2)} left.`}
            </p>
          )}
          <h3 className="settings-section__subtitle">{flags.length === 0 ? "No tickets need the manager" : "Needs the manager now"}</h3>
          {flags.length > 0 && (
            <ul className="manager-flags">
              {flags.map((flag) => (
                <li key={flag.fingerprint}>
                  <a href={`#/t/${flag.ticket_id}`}>{`#${flag.ticket_id}`}</a>
                  {` ${FLAG_LABELS[flag.kind]}: ${flag.detail}`}
                </li>
              ))}
            </ul>
          )}
          <h3 className="settings-section__subtitle">Recent sessions</h3>
          {sessions.length === 0 && <p className="muted small">No manager sessions yet.</p>}
          {sessions.map((session) => (
            <details key={`${session.at}-${session.model}`} className="manager-session">
              <summary>
                {`${new Date(session.at).toLocaleString()} · ${session.model || "model"} · $${session.cost_usd.toFixed(2)}`}
              </summary>
              {/* Agent text, shown as plain text only. */}
              <pre className="manager-session__summary">{session.summary || "(no summary)"}</pre>
            </details>
          ))}
        </>
      )}
    </section>
  );
}

function pick(s: BoardSettings): ManagerSettings {
  return {
    manager_enabled: s.manager_enabled,
    manager_build_cap: s.manager_build_cap,
    manager_stall_minutes: s.manager_stall_minutes,
    manager_daily_budget_usd: s.manager_daily_budget_usd,
    manager_budget_off: s.manager_budget_off,
  };
}
