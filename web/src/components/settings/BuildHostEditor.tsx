import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import type { BuildHost, BuildHostEdit } from "../../api/types";
import { BuildHostCapabilities } from "./BuildHostCapabilities";

const MIN_PARALLEL = 1;
const MAX_PARALLEL = 8;

/** Props for {@link BuildHostEditor}. */
export interface BuildHostEditorProps {
  host: BuildHost;
  isBusy: boolean;
  error: string | null;
  /** Sends only the fields the owner changed; resolves true once saved. */
  onSave: (edit: BuildHostEdit) => Promise<boolean>;
  onCancel: () => void;
}

type Dirty = { capacity: boolean; capabilities: boolean; enabled: boolean };
const CLEAN: Dirty = { capacity: false, capabilities: false, enabled: false };

/**
 * The collapsed-by-default configuration of an approved host. Fields the owner
 * has not touched follow the polled host; touched fields keep their draft
 * until saved, so a poll or a failed save never discards an edit.
 */
export function BuildHostEditor({ host, isBusy, error, onSave, onCancel }: BuildHostEditorProps) {
  const [capacity, setCapacity] = useState(String(host.max_parallel)),
    [capabilities, setCapabilities] = useState(host.capabilities),
    [isEnabled, setEnabled] = useState(host.enabled);
  const dirty = useRef<Dirty>({ ...CLEAN });
  const capacityId = useId();
  useEffect(() => {
    if (!dirty.current.capacity) {
      setCapacity(String(host.max_parallel));
    }
    if (!dirty.current.capabilities) {
      setCapabilities(host.capabilities);
    }
    if (!dirty.current.enabled) {
      setEnabled(host.enabled);
    }
  }, [host]);

  const n = Number(capacity);
  const isCapacityValid = Number.isInteger(n) && n >= MIN_PARALLEL && n <= MAX_PARALLEL;
  const hasChanges = dirty.current.capacity || dirty.current.capabilities || dirty.current.enabled;

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!isCapacityValid || !hasChanges) {
      return;
    }
    const edit: BuildHostEdit = {};
    if (dirty.current.capacity) {
      edit.max_parallel = n;
    }
    if (dirty.current.capabilities) {
      edit.capabilities = capabilities;
    }
    if (dirty.current.enabled) {
      edit.enabled = isEnabled;
    }
    if (await onSave(edit)) {
      dirty.current = { ...CLEAN };
    }
  };

  return (
    <form className="build-host__editor" onSubmit={(e) => void submit(e)} aria-label={`Edit ${host.name}`}>
      <fieldset className="build-host__fields" disabled={isBusy}>
        <div className="build-host__row">
          <div className="field">
            <label className="field__label" htmlFor={capacityId}>
              Max parallel builds
            </label>
            <input
              id={capacityId}
              className="input build-host__number"
              type="number"
              inputMode="numeric"
              min={MIN_PARALLEL}
              max={MAX_PARALLEL}
              step="1"
              value={capacity}
              aria-invalid={!isCapacityValid}
              onChange={(e) => {
                dirty.current.capacity = true;
                setCapacity(e.target.value);
              }}
            />
          </div>
          <label className="switch build-host__switch">
            <input
              type="checkbox"
              role="switch"
              className="switch__input"
              aria-label={`Enable ${host.name}`}
              checked={isEnabled}
              onChange={(e) => {
                dirty.current.enabled = true;
                setEnabled(e.target.checked);
              }}
            />
            <span className="switch__track" aria-hidden="true" />
            <span className="switch__label">Accept new builds</span>
          </label>
        </div>
        {!isCapacityValid && (
          <p className="field__hint field__hint--error" role="alert">
            Max parallel must be an integer from 1 through 8.
          </p>
        )}
        <BuildHostCapabilities
          values={capabilities}
          onChange={(values) => {
            dirty.current.capabilities = true;
            setCapabilities(values);
          }}
        />
      </fieldset>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="row-actions">
        <button type="button" className="button button--ghost button--small" onClick={onCancel}>
          Cancel
        </button>
        <button type="submit" className="button button--primary button--small" disabled={isBusy || !isCapacityValid || !hasChanges}>
          Save
        </button>
      </div>
    </form>
  );
}
