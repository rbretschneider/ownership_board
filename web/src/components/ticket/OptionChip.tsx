/** Props for {@link OptionChip}. */
export interface OptionChipProps {
  name: string;
  value: string;
  label: string;
  hint?: string;
  isSelected: boolean;
  isRecommended: boolean;
  onPick: (value: string) => void;
}

/** One answer choice styled as a chip, backed by a native radio. */
export function OptionChip({ name, value, label, hint, isSelected, isRecommended, onPick }: OptionChipProps) {
  return (
    <label className={`option${isSelected ? " option--selected" : ""}${isRecommended ? " option--recommended" : ""}`}>
      <input
        type="radio"
        className="visually-hidden"
        name={name}
        value={value}
        checked={isSelected}
        onChange={() => onPick(value)}
      />
      <span className="option__label">{label}</span>
      {isRecommended && <span className="option__tag">Recommended</span>}
      {hint !== undefined && <span className="option__hint">{hint}</span>}
    </label>
  );
}
