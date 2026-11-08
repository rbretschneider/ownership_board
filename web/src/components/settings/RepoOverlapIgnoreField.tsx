import {useId, useRef, useState} from "react";
import {useApi} from "../../api/ApiContext";
import type {Repo} from "../../api/types";

/** Person-managed exclusions from repository overlap scheduling. */
export function RepoOverlapIgnoreField({repo, onUpdated}: {repo: Repo; onUpdated: (repo: Repo) => void}) {
  const api = useApi();
  const id = useId();
  const [draft, setDraft] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const saving = useRef(false);
  const patterns = repo.overlap_ignore ?? [];
  const save = async () => {
    if (draft === null || saving.current) return;
    const entries = draft.split("\n").map(line => line.trim()).filter(Boolean);
    if (entries.length > 100 || entries.some(line => Array.from(line).length > 200)) {
      setError("Use at most 100 patterns, each up to 200 characters.");
      return;
    }
    saving.current = true;
    setBusy(true);
    setError(null);
    try {
      onUpdated(await api.editRepo(repo.id, {overlap_ignore: entries}));
      setDraft(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally { saving.current = false; setBusy(false); }
  };
  return <div className="repo-overlap-ignore">
    {draft === null ? <>
      <span className="small">Files that never block parallel builds: {patterns.length ? patterns.join("; ") : "None"}</span>{" "}
      <button type="button" className="button button--secondary" onClick={() => {setDraft(patterns.join("\n")); setError(null);}}>Edit shared files</button>
    </> : <fieldset disabled={busy}>
      <label htmlFor={id}>Files that never block parallel builds</label>
      <textarea id={id} className="input textarea" rows={5} value={draft} aria-describedby={`${id}-help`} onChange={e => {setDraft(e.target.value); setError(null);}} />
      <p id={`${id}-help`} className="muted small">Use for append-only registries such as error-code lists and message catalogs, where conflicts resolve mechanically. Enter one repository-relative pattern per line, up to 100 patterns of 200 characters each. Use forward slashes and Go path.Match glob syntax: *, ? and character classes match within a segment; ** does not match recursively.</p>
      {error && <p role="alert" className="field-error">{error}</p>}
      <button type="button" className="button" onClick={() => void save()}>Save</button>{" "}
      <button type="button" className="button button--secondary" onClick={() => {setDraft(null); setError(null);}}>Cancel</button>
    </fieldset>}
  </div>;
}
