import { X } from "lucide-react";
import { useId, useRef, useState, type KeyboardEvent } from "react";

const CAPABILITY_PATTERN = /^[a-z][a-z0-9_-]{0,31}$/;
const MAX_CAPABILITIES = 32;

/** Props for {@link BuildHostCapabilities}. */
export interface BuildHostCapabilitiesProps {
  values: string[];
  onChange: (values: string[]) => void;
  disabled?: boolean;
}

/**
 * Capability tokens as an inline chip input: type a token and press Enter (or
 * comma) to add it, Backspace on an empty field removes the last chip, and a
 * valid token left in the field is added when focus leaves it.
 */
export function BuildHostCapabilities({ values, onChange, disabled = false }: BuildHostCapabilitiesProps) {
  const [draft, setDraft] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const labelId = useId(),
    hintId = useId();
  const token = draft.trim();
  const isValid = CAPABILITY_PATTERN.test(token) && values.length < MAX_CAPABILITIES && !values.includes(token);
  const isInvalid = token !== "" && !isValid;

  const commit = () => {
    if (isValid) {
      onChange([...values, token]);
      setDraft("");
    }
  };
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter" || event.key === ",") {
      event.preventDefault();
      commit();
      return;
    }
    if (event.key === "Backspace" && draft === "" && values.length > 0) {
      onChange(values.slice(0, -1));
    }
  };

  return (
    <div className="field">
      <span className="field__label" id={labelId}>
        Capabilities
      </span>
      <div
        className={`chip-input${isInvalid ? " chip-input--invalid" : ""}${disabled ? " chip-input--disabled" : ""}`}
        onClick={() => inputRef.current?.focus()}
      >
        {values.map((value) => (
          <span className="chip-input__chip" key={value}>
            {value}
            <button
              type="button"
              className="chip-input__remove"
              disabled={disabled}
              aria-label={`Remove ${value}`}
              onClick={() => onChange(values.filter((v) => v !== value))}
            >
              <X size={12} aria-hidden="true" />
            </button>
          </span>
        ))}
        <input
          ref={inputRef}
          className="chip-input__field"
          aria-labelledby={labelId}
          aria-describedby={hintId}
          aria-invalid={isInvalid}
          placeholder={values.length === 0 ? "e.g. dotnet" : "Add…"}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          enterKeyHint="done"
          value={draft}
          disabled={disabled}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKeyDown}
          onBlur={commit}
        />
      </div>
      <span id={hintId} className={isInvalid ? "field__hint field__hint--error" : "field__hint"} role={isInvalid ? "status" : undefined}>
        {isInvalid
          ? "Use a unique lowercase token, up to 32 characters (maximum 32 capabilities)."
          : "Type a capability and press Enter."}
      </span>
    </div>
  );
}
