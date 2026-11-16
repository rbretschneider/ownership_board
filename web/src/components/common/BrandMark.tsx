/** Size of the mark in the page headers. */
export const HEADER_MARK_SIZE = 32;

/** Raster renders of the twin-piston emblem, by pixel width. */
const BRAND_SOURCES = "/brand-64.png 64w, /brand-96.png 96w, /brand-128.png 128w, /brand-192.png 192w";

/**
 * The app's mark in the title bar: the twin-piston lightning emblem, served as
 * high-resolution renders so it stays sharp at every pixel density. The
 * single-piston SVG (/icon.svg) is the favicon, where the twin art does not
 * read.
 */
export function BrandMark({ size = HEADER_MARK_SIZE }: { size?: number }) {
  return (
    <img
      className="brand-mark"
      src="/brand-64.png"
      srcSet={BRAND_SOURCES}
      sizes={`${size}px`}
      width={size}
      height={size}
      alt=""
      aria-hidden="true"
      decoding="async"
    />
  );
}
