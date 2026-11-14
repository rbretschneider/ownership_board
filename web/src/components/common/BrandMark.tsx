/**
 * The app's logo: a pixel-art white hard hat (the architect's hat on a job site)
 * on a blueprint of a high-rise, with a drafting compass.
 */
export function BrandMark({ size = 24 }: { size?: number }) {
  return <img src="/icon-192.png" width={size} height={size} alt="" aria-hidden="true" className="brand-mark" />;
}
