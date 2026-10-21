import { Plus } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useApi } from "../../api/ApiContext";
import type { Repo } from "../../api/types";
import { Spinner } from "../common/Spinner";
import { useToast } from "../toast/ToastContext";

const FULL_NAME_PATTERN = /^[\w.-]+(\/[\w.-]+)+$/;

/** Adds a repository by hand (for providers without a sync source). */
export function ManualRepoForm({ onAdded }: { onAdded: (repo: Repo) => void }) {
  const api = useApi();
  const toast = useToast();
  const [isOpen, setOpen] = useState(false);
  const [fullName, setFullName] = useState("");
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!FULL_NAME_PATTERN.test(fullName.trim())) {
      setError("Use owner/name, for example acme/api.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const repo = await api.addRepo({ full_name: fullName.trim(), url: url.trim(), description: "", color: "" });
      onAdded(repo);
      toast.success(`Added ${repo.full_name}`);
      setFullName("");
      setUrl("");
      setOpen(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };

  if (!isOpen) {
    return (
      <button type="button" className="link-button manual-repo__toggle" onClick={() => setOpen(true)}>
        <Plus size={14} aria-hidden="true" />
        Add a repo by hand
      </button>
    );
  }
  return (
    <form className="inline-form" onSubmit={(e) => void submit(e)} noValidate>
      <div className="field">
        <label className="field__label" htmlFor="manual-repo-name">Full name</label>
        <input
          id="manual-repo-name"
          className="input"
          value={fullName}
          onChange={(e) => setFullName(e.target.value)}
          placeholder="owner/name"
          aria-invalid={error !== null}
          aria-describedby={error !== null ? "manual-repo-error" : undefined}
          spellCheck={false}
          autoComplete="off"
          autoFocus
        />
      </div>
      <div className="field">
        <label className="field__label" htmlFor="manual-repo-url">URL</label>
        <input id="manual-repo-url" className="input" type="url" inputMode="url" value={url} onChange={(e) => setUrl(e.target.value)} />
      </div>
      {error !== null && <p id="manual-repo-error" className="form-error" role="alert">{error}</p>}
      <div className="row-actions">
        <button type="button" className="button button--secondary" onClick={() => setOpen(false)}>Cancel</button>
        <button type="submit" className="button button--primary" disabled={isBusy}>
          {isBusy && <Spinner label="Adding" />}
          Add repo
        </button>
      </div>
    </form>
  );
}
