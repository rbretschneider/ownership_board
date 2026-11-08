import { useState } from "react";
export function BuildHostCapabilities({
  values,
  onChange,
  disabled = false,
}: {
  values: string[];
  onChange: (v: string[]) => void;
  disabled?: boolean;
}) {
  const [draft, setDraft] = useState("");
  const valid = /^[a-z][a-z0-9_-]{0,31}$/.test(draft) && values.length < 32 && !values.includes(draft);
  return (
    <div className="build-host-capabilities">
      <span>Capabilities</span>
      <div className="build-host-chips">
        {values.map((v) => (
          <span className="build-host-chip" key={v}>
            {v}
            <button
              type="button"
              disabled={disabled}
              aria-label={`Remove ${v}`}
              onClick={() => onChange(values.filter((x) => x !== v))}
            >
              ×
            </button>
          </span>
        ))}
      </div>
      <label>
        New capability{" "}
        <input
          className="input"
          value={draft}
          disabled={disabled}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              if (valid) {
                onChange([...values, draft]);
                setDraft("");
              }
            }
          }}
        />
      </label>
      <button
        type="button"
        disabled={disabled || !valid}
        onClick={() => {
          onChange([...values, draft]);
          setDraft("");
        }}
      >
        Add capability
      </button>
      {draft && !valid && (
        <span role="status">Use a unique lowercase token, up to 32 characters (maximum 32 capabilities).</span>
      )}
    </div>
  );
}
