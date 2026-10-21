import { Moon, Sun } from "lucide-react";
import { useTheme, type Theme } from "../../hooks/useTheme";

const THEMES: { value: Theme; label: string; Icon: typeof Sun }[] = [
  { value: "dark", label: "Dark", Icon: Moon },
  { value: "light", label: "Light", Icon: Sun },
];

/** Theme choice (stored in this browser). */
export function AppearanceSection() {
  const [theme, setTheme] = useTheme();
  return (
    <section className="settings-section" aria-labelledby="appearance-title">
      <h3 id="appearance-title" className="settings-section__title">Appearance</h3>
      <fieldset className="segmented">
        <legend className="visually-hidden">Theme</legend>
        {THEMES.map(({ value, label, Icon }) => (
          <label key={value} className={`segmented__option${theme === value ? " segmented__option--active" : ""}`}>
            <input type="radio" name="theme" value={value} checked={theme === value} onChange={() => setTheme(value)} className="visually-hidden" />
            <Icon size={16} aria-hidden="true" />
            {label}
          </label>
        ))}
      </fieldset>
    </section>
  );
}
