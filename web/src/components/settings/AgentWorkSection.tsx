import { useCallback, useEffect, useId, useRef, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BoardSettings } from "../../api/types";
import { Minus, Plus } from "lucide-react";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { useToast } from "../toast/ToastContext";

/** Board-wide limits on autonomous agent work. */
export function AgentWorkSection() {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [settings, setSettings] = useState<BoardSettings | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isSaving, setSaving] = useState(false);
  const [isWindowsSaving, setWindowsSaving] = useState(false);
  const desired = useRef(0);
  const confirmed = useRef<BoardSettings | null>(null);
  const saving = useRef(false);

  const load = useCallback(async () => {
    setError(null);
    try {
      const value = await api.settings();
      confirmed.current = value;
      desired.current = value.codex_workers;
      setSettings(value);
    } catch (cause) {
      setSettings(null);
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [api]);

  useEffect(() => {
    void load();
  }, [load]);

  const changeWorkers = async (value: number) => {
    if (isWindowsSaving) return;
    const current = confirmed.current;
    if (!current || !Number.isInteger(value)) return;
    desired.current = Math.min(current.codex_worker_limit, Math.max(0, value));
    setSettings({...current, codex_workers: desired.current});
    if (saving.current) return;
    saving.current = true;
    setSaving(true);
    try {
      while (desired.current !== confirmed.current!.codex_workers) {
        const target = desired.current;
        const result = await api.editSettings({codex_workers: target, autonomous_in_flight_cap: Math.max(1, target)});
        confirmed.current = result;
        setSettings({...result, codex_workers: desired.current});
      }
    } catch (cause) {
      // A dropped response can follow a committed PATCH. Read the server
      // instead of showing the old count as if it were still authoritative.
      try {
        const actual = await api.settings();
        confirmed.current = actual;
        desired.current = actual.codex_workers;
        setSettings(actual);
      } catch {
        setSettings(null);
        setError("Worker count is unconfirmed. Reload to see the current setting.");
      }
      toast.error(`Couldn't change workers: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      saving.current = false;
      setSaving(false);
    }
  };

  const changeWindows = async (enabled: boolean) => {
    setWindowsSaving(true);
    try {
      const actual = await api.editSettings({windows_runner_enabled: enabled});
      confirmed.current = actual;
      setSettings(actual);
    } catch (cause) {
      await load();
      toast.error(`Couldn't change Windows runner: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setWindowsSaving(false);
    }
  };

  return (
    <section className="settings-section" aria-labelledby={`${id}-title`}>
      <h3 id={`${id}-title`} className="settings-section__title">Agent work</h3>
      {error !== null && <ErrorState message={`Couldn't load the settings: ${error}`} onRetry={() => void load()} />}
      {error === null && settings === null && <LoadingState label="Loading settings" />}
      {settings !== null && (
        <div className="field">
          <label className="switch" htmlFor={`${id}-windows`}>
            <input id={`${id}-windows`} type="checkbox" role="switch" className="switch__input" checked={settings.windows_runner_enabled} disabled={isSaving || isWindowsSaving} onChange={(e) => void changeWindows(e.target.checked)} />
            <span className="switch__track" aria-hidden="true" />
            <span className="switch__label">Enable Windows runner</span>
          </label>
          <p className="muted small">Allow Windows checks for every repository on this board. Turning this off stops Windows builds and holds new jobs until you enable it again. Runs hidden at low priority, with at most two build/test jobs.</p>
          <label className="field__label" htmlFor={`${id}-workers`}>Codex workers</label>
          <div className="worker-stepper">
            <button type="button" className="button button--secondary" aria-label="Fewer Codex workers" disabled={isWindowsSaving || settings.codex_workers === 0} onClick={() => void changeWorkers(desired.current - 1)}><Minus size={18} aria-hidden="true" /></button>
            <input id={`${id}-workers`} className="input" type="number" disabled={isWindowsSaving} min={0} max={settings.codex_worker_limit} step={1} value={settings.codex_workers} onChange={(e) => { if (e.target.value !== "") void changeWorkers(Number(e.target.value)); }} />
            <button type="button" className="button button--secondary" aria-label="More Codex workers" disabled={isWindowsSaving || settings.codex_workers >= settings.codex_worker_limit} onClick={() => void changeWorkers(desired.current + 1)}><Plus size={18} aria-hidden="true" /></button>
            <span className="muted small" role="status">{isSaving ? "Saving…" : settings.codex_workers === 0 ? "Paused" : `Up to ${settings.codex_workers} working`} · max {settings.codex_worker_limit}</span>
          </div>
          <p className="muted small">Workers share one account and board token. Each is capped at 3 CPUs and 8 GB memory. More workers start within 30 seconds; scaling down lets current assignments finish. Set 0 to pause new work.</p>
        </div>
      )}
    </section>
  );
}
