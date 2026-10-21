import { Globe, Lock } from "lucide-react";
import { useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Repo } from "../../api/types";
import { useToast } from "../toast/ToastContext";

/**
 * A repo's visibility to agents, plus the person-only "Treat as public"
 * switch for repos that may be open-sourced later. Public means agents keep
 * board content out of its commits and PRs.
 */
export function RepoVisibilityToggle({ repo, onUpdated }: { repo: Repo; onUpdated: (repo: Repo) => void }) {
  const api = useApi();
  const toast = useToast();
  const id = useId();
  const [isBusy, setBusy] = useState(false);
  const isManual = repo.provider === "manual";
  const isPublic = repo.visibility === "public";

  const toggle = async (next: boolean) => {
    setBusy(true);
    try {
      onUpdated(await api.editRepo(repo.id, { treat_as_public: next }));
      toast.success(next ? `${repo.full_name} is now treated as public` : `${repo.full_name} follows its provider again`);
    } catch (cause) {
      toast.error(`Couldn't change ${repo.full_name}: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="repo-visibility">
      <span className={`badge badge--${isPublic ? "public" : "private"}`}>
        {isPublic ? <Globe size={12} aria-hidden="true" /> : <Lock size={12} aria-hidden="true" />}
        {isPublic ? "Public" : "Private"}
      </span>
      <label className="switch" htmlFor={`${id}-public`}>
        <input
          id={`${id}-public`}
          type="checkbox"
          role="switch"
          className="switch__input"
          checked={isManual || repo.treat_as_public}
          disabled={isManual || isBusy}
          onChange={(e) => void toggle(e.target.checked)}
          aria-describedby={isManual ? `${id}-hint` : undefined}
        />
        <span className="switch__track" aria-hidden="true" />
        <span className="switch__label">Treat as public</span>
      </label>
      {isManual && <span id={`${id}-hint`} className="muted small">Manual repos are always treated as public.</span>}
    </div>
  );
}
