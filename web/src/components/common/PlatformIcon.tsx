import type { Platform, PlatformStatus } from "../../lib/progress";

/* Small, hand-drawn platform glyphs (12×12, currentColor) in the board's pixel style. */
export function PlatformGlyph({ platform }: { platform: Platform }) {
  switch (platform) {
    case "windows":
      return (
        <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
          <path d="M0.5 1.6 5.2 0.9v4.6H0.5zM6 0.8 11.5 0v5.5H6zM0.5 6.3h4.7v4.6l-4.7-.7zM6 6.3h5.5V12L6 11.2z" fill="currentColor" />
        </svg>
      );
    case "linux":
      // A penguin: head and body, white belly, two eyes, beak and feet.
      return (
        <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
          <path d="M6 0.4C4.3 0.4 3.5 1.8 3.5 3.4c0 .9-.4 1.6-1 2.5C1.6 7.2 1.3 8.6 1.9 9.6l-.9 1.2h3.2c.5.3 1.1.5 1.8.5s1.3-.2 1.8-.5H11l-.9-1.2c.6-1 .3-2.4-.6-3.7-.6-.9-1-1.6-1-2.5C8.5 1.8 7.7.4 6 .4z" fill="currentColor" />
          <path d="M6 4.6c-1.2 0-2 1.5-2 3.2S4.8 10.6 6 10.6s2-1.1 2-2.8-.8-3.2-2-3.2z" fill="var(--surface, #15171c)" />
          <circle cx="5.1" cy="2.9" r="0.45" fill="var(--surface, #15171c)" />
          <circle cx="6.9" cy="2.9" r="0.45" fill="var(--surface, #15171c)" />
          <path d="M5.2 3.9h1.6L6 4.7z" fill="var(--surface, #15171c)" />
        </svg>
      );
    case "macos":
      // An apple with its bite and leaf.
      return (
        <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
          <path d="M8.1 0.3c.1.8-.2 1.6-.7 2.1-.5.6-1.2.9-1.9.9-.1-.8.2-1.6.7-2.1.5-.5 1.2-.9 1.9-.9z" fill="currentColor" />
          <path d="M10.6 8.3c-.3.7-.5 1-.9 1.6-.5.8-1.3 1.8-2.2 1.8-.8 0-1-.5-2.1-.5s-1.4.5-2.2.5c-.9 0-1.6-.9-2.1-1.7C0 8 -.2 5.4 1.1 4.1c.6-.7 1.5-1 2.3-1 .9 0 1.4.5 2.2.5.7 0 1.2-.5 2.2-.5.7 0 1.5.4 2.1 1.1-1.8 1-1.5 3.6.7 4.1z" fill="currentColor" />
        </svg>
      );
  }
}

const STATE_WORDS = { passed: "passed", failed: "failed", running: "running" } as const;

/** One platform's result as a colored icon; the full wording is the tooltip and accessible name. */
export function PlatformIcon({ status }: { status: PlatformStatus }) {
  return (
    <span className={`platform platform--${status.state}`} role="img" title={status.title} aria-label={status.title}
      data-platform={status.platform} data-state={STATE_WORDS[status.state]}>
      <PlatformGlyph platform={status.platform} />
    </span>
  );
}
