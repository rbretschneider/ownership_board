/** Size of the mark in the page headers: large enough to read the bolts. */
export const HEADER_MARK_SIZE = 32;

/**
 * The app's mark: the simplified lightning engine, the V of a V8's heads with
 * a bolt firing straight up out of each. The full emblem (/emblem.svg) is for
 * large sizes; below 96 px only this silhouette reads.
 */
export function BrandMark({ size = HEADER_MARK_SIZE }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" focusable="false" className="brand-mark">
      <rect className="brand-mark__tile" width="64" height="64" rx="12" fill="#0e1116" />
      <path d="M14 22 L30 14 L34 30 L18 36 Z M50 22 L34 14 L30 30 L46 36 Z" fill="#c9d0db" />
      <rect x="20" y="32" width="24" height="18" rx="3" fill="#8a93a3" />
      {size >= 40 && <circle cx="32" cy="46" r="5" fill="#e6e9ef" />}
      <path d="M20 18 L12 8 L17 9 L10 0 L25 12 L20 11 L26 17 Z M44 18 L52 8 L47 9 L54 0 L39 12 L44 11 L38 17 Z" fill="#4f8cff" />
    </svg>
  );
}
