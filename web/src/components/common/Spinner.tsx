/** A small inline spinner. `label` is announced to screen readers. */
export function Spinner({ label = "Loading", size = 16 }: { label?: string; size?: number }) {
  return (
    <span className="spinner" role="status" style={{ width: size, height: size }}>
      <span className="visually-hidden">{label}</span>
    </span>
  );
}
