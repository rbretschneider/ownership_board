import { useCallback, useEffect, useState } from "react";
import { preferences, STORAGE_KEYS } from "../lib/storage";

/** The two supported color themes. Dark is the default. */
export type Theme = "dark" | "light";

const THEME_COLORS: Record<Theme, string> = { dark: "#0e0f13", light: "#f6f7f9" };

function readTheme(): Theme {
  return preferences.read(STORAGE_KEYS.theme, "dark") === "light" ? "light" : "dark";
}

/** Applies a theme to the document and the browser chrome color. */
export function applyTheme(theme: Theme): void {
  document.documentElement.dataset["theme"] = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", THEME_COLORS[theme]);
}

/** The persisted theme and a setter that applies and stores it. */
export function useTheme(): [Theme, (theme: Theme) => void] {
  const [theme, setThemeState] = useState<Theme>(readTheme);
  useEffect(() => {
    applyTheme(theme);
  }, [theme]);
  const setTheme = useCallback((next: Theme) => {
    preferences.write(STORAGE_KEYS.theme, next);
    setThemeState(next);
  }, []);
  return [theme, setTheme];
}

/** Applies the stored theme before React renders, avoiding a flash. */
export function applyStoredTheme(): void {
  applyTheme(readTheme());
}
