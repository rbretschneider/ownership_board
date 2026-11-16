import { useEffect, useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { RunnerName } from "../../api/types";
import { useToast } from "../toast/ToastContext";

/** The longest display name the board accepts. */
export const MAX_RUNNER_NAME_LENGTH = 32;

/** Props for {@link RunnerNameRow}. */
export interface RunnerNameRowProps {
  runner: RunnerName;
  /** Called after a rename the server accepted. */
  onRenamed: () => void;
}

/** One named runner: its worker id and an editable display name. */
export function RunnerNameRow({ runner, onRenamed }: RunnerNameRowProps) {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [draft, setDraft] = useState(runner.display_name);
  const [isSaving, setSaving] = useState(false);
  useEffect(() => setDraft(runner.display_name), [runner.display_name]);
  const trimmed = draft.trim();
  const isChanged = trimmed !== "" && trimmed !== runner.display_name;

  const save = async () => {
    setSaving(true);
    try {
      const renamed = await api.renameRunner(runner.worker, trimmed);
      toast.success(`${runner.worker} is now ${renamed.display_name}`);
      onRenamed();
    } catch (cause) {
      toast.error(`Couldn't rename ${runner.worker}: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setSaving(false);
    }
  };

  return (
    <form className="runner-name-row" onSubmit={(event) => { event.preventDefault(); if (isChanged) void save(); }}>
      <label className="field__label" htmlFor={`${id}-name`}>
        {runner.worker} <span className="muted small">({runner.family})</span>
      </label>
      <div className="runner-name-row__controls">
        <input id={`${id}-name`} className="input" value={draft} maxLength={MAX_RUNNER_NAME_LENGTH} disabled={isSaving}
          onChange={(event) => setDraft(event.target.value)} />
        <button type="submit" className="button button--secondary" disabled={!isChanged || isSaving}>
          {isSaving ? "Saving…" : "Rename"}
        </button>
      </div>
    </form>
  );
}
