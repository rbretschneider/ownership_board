import { useCallback, useEffect, useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BoardSettings } from "../../api/types";
import { IN_FLIGHT_CAP_CHOICES } from "../../lib/settings";
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

  const load = useCallback(async () => {
    setError(null);
    try {
      setSettings(await api.settings());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [api]);

  useEffect(() => {
    void load();
  }, [load]);

  const changeCap = async (value: number) => {
    setSaving(true);
    try {
      setSettings(await api.editSettings({ autonomous_in_flight_cap: value }));
      toast.success(value === 1 ? "One autonomous ticket at a time" : `Up to ${value} autonomous tickets at a time`);
    } catch (cause) {
      toast.error(`Couldn't change the limit: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="settings-section" aria-labelledby={`${id}-title`}>
      <h3 id={`${id}-title`} className="settings-section__title">Agent work</h3>
      {error !== null && <ErrorState message={`Couldn't load the settings: ${error}`} onRetry={() => void load()} />}
      {error === null && settings === null && <LoadingState label="Loading settings" />}
      {settings !== null && (
        <div className="field">
          <label className="field__label" htmlFor={`${id}-cap`}>Autonomous tickets in progress at once</label>
          <select
            id={`${id}-cap`}
            className="select"
            value={settings.autonomous_in_flight_cap}
            disabled={isSaving}
            onChange={(e) => void changeCap(Number(e.target.value))}
          >
            {IN_FLIGHT_CAP_CHOICES.map((value) => (
              <option key={value} value={value}>{value}</option>
            ))}
          </select>
          <p className="muted small">
            While this many autonomous tickets are in design or build, agents start no other autonomous ticket from
            Ready. Other work still flows, and the queue itself has no limit.
          </p>
        </div>
      )}
    </section>
  );
}
