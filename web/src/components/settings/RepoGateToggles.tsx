import { useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Repo } from "../../api/types";
import { useToast } from "../toast/ToastContext";

type Gate = "require_spec_approval" | "require_design_approval" | "allow_autonomous";

const GATES: readonly { field: Gate; label: string }[] = [
  { field: "require_spec_approval", label: "Approve spec" },
  { field: "require_design_approval", label: "Approve design" },
  { field: "allow_autonomous", label: "Allow autonomous" },
];

/**
 * A repo's checkpoints: whether agents wait for the owner to approve the spec
 * before building and the design before implementing, and whether a delegate
 * agent may file autonomous tickets that skip both. Toggles apply at once
 * and revert if the server refuses.
 */
export function RepoGateToggles({ repo, onUpdated }: { repo: Repo; onUpdated: (repo: Repo) => void }) {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [busy, setBusy] = useState<Gate | null>(null);

  const toggle = async (field: Gate, label: string, next: boolean) => {
    setBusy(field);
    onUpdated({ ...repo, [field]: next });
    try {
      onUpdated(await api.editRepo(repo.id, { [field]: next }));
    } catch (cause) {
      onUpdated(repo);
      toast.error(`Couldn't change "${label}" for ${repo.full_name}: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="repo-gates" role="group" aria-labelledby={`${id}-title`} aria-describedby={`${id}-hint`}>
      <span id={`${id}-title`} className="visually-hidden">Checkpoints for {repo.full_name}</span>
      {GATES.map(({ field, label }) => (
        <label key={field} className="switch" htmlFor={`${id}-${field}`}>
          <input
            id={`${id}-${field}`}
            type="checkbox"
            role="switch"
            className="switch__input"
            checked={repo[field]}
            disabled={busy !== null}
            onChange={(e) => void toggle(field, label, e.target.checked)}
          />
          <span className="switch__track" aria-hidden="true" />
          <span className="switch__label">{label}</span>
        </label>
      ))}
      <span id={`${id}-hint`} className="muted small">
        Agents wait for you before building/after designing. Allow autonomous lets a delegate agent file tickets
        that skip both.
      </span>
    </div>
  );
}
