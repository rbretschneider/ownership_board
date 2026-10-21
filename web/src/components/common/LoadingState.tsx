import { Spinner } from "./Spinner";

/** A centered spinner with a visible label. */
export function LoadingState({ label }: { label: string }) {
  return (
    <div className="loading-state">
      <Spinner label={label} size={22} />
      <span aria-hidden="true">{label}…</span>
    </div>
  );
}
