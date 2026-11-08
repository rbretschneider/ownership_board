import { useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { DeployMode, Repo, RepoEdit } from "../../api/types";
import { useToast } from "../toast/ToastContext";

type Gate = "require_spec_approval" | "require_design_approval" | "allow_agent_merge";

const GATES: readonly { field: Gate; label: string }[] = [
  { field: "require_spec_approval", label: "Approve spec" },
  { field: "require_design_approval", label: "Approve design" },
  { field: "allow_agent_merge", label: "Auto-merge green PRs" },
];

/**
 * A repo's checkpoints: whether agents wait for the owner to approve the spec
 * before building and the design before implementing. Toggles apply at once
 * and revert if the server refuses.
 */
export function RepoGateToggles({ repo, onUpdated }: { repo: Repo; onUpdated: (repo: Repo) => void }) {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [busy, setBusy] = useState<Gate | "deploy_mode" | null>(null);

  const save = async (field: Gate | "deploy_mode", label: string, patch: RepoEdit) => {
    setBusy(field);
    onUpdated({ ...repo, ...patch });
    try {
      onUpdated(await api.editRepo(repo.id, patch));
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
            onChange={(e) => void save(field, label, { [field]: e.target.checked })}
          />
          <span className="switch__track" aria-hidden="true" />
          <span className="switch__label">{label}</span>
        </label>
      ))}
      <div className="repo-deploys">
        <label htmlFor={`${id}-deploys`}>Deploys</label>
        <select className="input select" id={`${id}-deploys`} value={repo.deploy_mode ?? "off"} disabled={busy !== null}
          aria-describedby={`${id}-deploys-hint`}
          onChange={e => void save("deploy_mode", "Deploys", { deploy_mode: e.target.value as DeployMode })}>
          <option value="off">Off</option>
          <option value="agent">Agent deploys</option>
        </select>
        <span className="field__hint" id={`${id}-deploys-hint`}>With Off, merged work goes straight to Deployed.</span>
      </div>
      <span id={`${id}-hint`} className="visually-hidden">Enabled approval checkpoints wait for you; auto-merge applies only to green PRs.</span>
    </div>
  );
}
