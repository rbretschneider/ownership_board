/** localStorage keys used by the UI. */
export const STORAGE_KEYS = {
  theme: "ob.theme",
  lastRepo: "ob.lastRepo",
  lastKind: "ob.lastKind",
  repoFilter: "ob.repoFilter",
} as const;

/**
 * Per-browser preferences. Storage may be unavailable (private windows,
 * blocked site data); preferences are conveniences, so a failure falls back to
 * the default rather than breaking the page.
 */
export const preferences = {
  /** Reads a stored string, or `fallback` when absent or unavailable. */
  read(key: string, fallback: string): string {
    try {
      return window.localStorage.getItem(key) ?? fallback;
    } catch {
      return fallback;
    }
  },

  /** Stores a string; silently skipped when storage is unavailable. */
  write(key: string, value: string): void {
    try {
      window.localStorage.setItem(key, value);
    } catch {
      // Preferences are optional; the in-memory state still applies.
    }
  },
};
