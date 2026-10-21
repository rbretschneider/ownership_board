/** The app's geometric mark: a ball passing between two lanes. */
export function BrandMark({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" className="brand-mark">
      <rect x="3" y="5" width="11" height="22" rx="3.5" fill="currentColor" opacity="0.28" />
      <rect x="18" y="5" width="11" height="22" rx="3.5" fill="currentColor" opacity="0.28" />
      <circle cx="16" cy="16" r="5.5" fill="var(--accent)" />
    </svg>
  );
}
