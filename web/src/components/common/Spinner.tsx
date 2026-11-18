import { EngineSpinner } from "./EngineSpinner";

/**
 * The app's working spinner, inline. `label` is announced to screen readers.
 * It draws Turbine's engine (EngineSpinner); the Claude and Codex miners stay
 * on agent work.
 */
export function Spinner({ label = "Loading", size = 16 }: { label?: string; size?: number }) {
  return (
    <span className="spinner" role="status" style={{ width: size, height: size }}>
      <EngineSpinner size={size} />
      <span className="visually-hidden">{label}</span>
    </span>
  );
}
