import { useId } from "react";

/**
 * Turbine's working spinner: the twin-piston emblem with each piston stroking
 * down its rod and firing lightning at the top, left then right.
 *
 * Rendered inline because an SVG loaded through <img> cannot pull in the
 * emblem raster it is cut from. Every copy gets its own ids, so any number can
 * share a page. The geometry matches public/spinner.svg (measured on the
 * 512 px emblem); the motion lives in components.css (.engine-spinner).
 */
export function EngineSpinner({ size = 16, label }: { size?: number; label?: string }) {
  const raw = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  const id = (part: string) => `es-${raw}-${part}`;
  const url = (part: string) => `url(#${id(part)})`;
  const bodyLeft = "M195 387L141 297L106 318L45 218L63 194L154 139L191 130L254 235L209 262L256 352Z";
  const bodyRight = "M317 387L371 297L406 318L467 218L449 194L358 139L321 130L258 235L303 262L256 352Z";
  const boltLeft = "M0 0H256V77L0 232Z";
  const boltRight = "M512 0H256V77L512 232Z";
  return (
    <svg
      className="engine-spinner"
      viewBox="0 0 512 512"
      width={size}
      height={size}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      focusable="false"
    >
      <defs>
        <image id={id("emblem")} href="/brand-192.png" width="512" height="512" />
        <clipPath id={id("hub")}><circle cx="256" cy="397" r="104" /></clipPath>
        <clipPath id={id("piston-left")}><path d={bodyLeft} /></clipPath>
        <clipPath id={id("piston-right")}><path d={bodyRight} /></clipPath>
        <clipPath id={id("bolt-left")}><path d={boltLeft} /></clipPath>
        <clipPath id={id("bolt-right")}><path d={boltRight} /></clipPath>
        <mask id={id("glow-left")} maskUnits="userSpaceOnUse" x="0" y="0" width="512" height="512">
          <rect width="256" height="300" fill="#fff" />
          <path d={bodyLeft} fill="#000" />
          <path d={boltLeft} fill="#000" />
          <circle cx="256" cy="397" r="104" fill="#000" />
        </mask>
        <mask id={id("glow-right")} maskUnits="userSpaceOnUse" x="0" y="0" width="512" height="512">
          <rect x="256" width="256" height="300" fill="#fff" />
          <path d={bodyRight} fill="#000" />
          <path d={boltRight} fill="#000" />
          <circle cx="256" cy="397" r="104" fill="#000" />
        </mask>
      </defs>
      <g className="engine-spinner__left">
        <g className="engine-spinner__glow"><use href={`#${id("emblem")}`} mask={url("glow-left")} /></g>
        <use href={`#${id("emblem")}`} clipPath={url("piston-left")} />
        <g className="engine-spinner__bolt"><use href={`#${id("emblem")}`} clipPath={url("bolt-left")} /></g>
      </g>
      <g className="engine-spinner__right">
        <g className="engine-spinner__glow"><use href={`#${id("emblem")}`} mask={url("glow-right")} /></g>
        <use href={`#${id("emblem")}`} clipPath={url("piston-right")} />
        <g className="engine-spinner__bolt"><use href={`#${id("emblem")}`} clipPath={url("bolt-right")} /></g>
      </g>
      <use href={`#${id("emblem")}`} clipPath={url("hub")} />
    </svg>
  );
}

/**
 * The Board's original 3×3 build matrix. Kept for reference and reuse; the
 * cards and host slots now show {@link EngineSpinner} instead.
 */
export function PixelLoader({ className = "" }: { className?: string }) {
  return <span className={`pixel-loader ${className}`.trim()} aria-hidden="true">{Array.from({ length: 9 }, (_, i) => <i key={i} />)}</span>;
}
