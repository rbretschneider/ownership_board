import { BookOpen, Bug } from "lucide-react";
import type { Kind } from "../../api/types";

const OPTIONS: { value: Kind; label: string; Icon: typeof Bug }[] = [
  { value: "story", label: "Story", Icon: BookOpen },
  { value: "bug", label: "Bug", Icon: Bug },
];

/** Segmented Story/Bug control built on native radios. */
export function KindToggle({ value, onChange }: { value: Kind; onChange: (kind: Kind) => void }) {
  return (
    <fieldset className="segmented">
      <legend className="visually-hidden">Type</legend>
      {OPTIONS.map(({ value: option, label, Icon }) => (
        <label key={option} className={`segmented__option${value === option ? " segmented__option--active" : ""}`}>
          <input
            type="radio"
            name="kind"
            value={option}
            checked={value === option}
            onChange={() => onChange(option)}
            className="visually-hidden"
          />
          <Icon size={16} aria-hidden="true" />
          {label}
        </label>
      ))}
    </fieldset>
  );
}
