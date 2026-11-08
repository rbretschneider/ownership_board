import {useRef, useState} from "react";
import {useApi} from "../../api/ApiContext";
import type {BuildHostOS, Repo, RepoBuildTarget} from "../../api/types";

type Draft = {os: BuildHostOS; preset: RepoBuildTarget["preset"]; capabilities: string};
const fromTargets = (targets: RepoBuildTarget[]): Draft[] => targets.map(t => ({...t, capabilities: t.capabilities.join(", ")}));

function parse(draft: Draft[]): RepoBuildTarget[] {
  if (draft.length > 32) throw new Error("Use at most 32 build targets.");
  const seen = new Set<string>();
  return draft.map(t => {
    const capabilities = t.capabilities.trim().split(/[\s,]+/).filter(Boolean);
    if (capabilities.length > 32 || capabilities.some(c => !/^[a-z][a-z0-9_-]{0,31}$/.test(c))) {
      throw new Error("Capabilities must be at most 32 lowercase tokens, each up to 32 characters.");
    }
    const key = JSON.stringify([t.os, t.preset, [...new Set(capabilities)].sort()]);
    if (seen.has(key)) throw new Error("Remove duplicate build targets.");
    seen.add(key);
    return {...t, capabilities};
  });
}

/** Explicitly save a repository's required platforms and toolchains. */
export function RepoBuildTargetsField({repo, onUpdated}: {repo: Repo; onUpdated: (repo: Repo) => void}) {
  const api = useApi();
  const [draft, setDraft] = useState<Draft[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const saving = useRef(false);
  const update = (index: number, patch: Partial<Draft>) => {
    setDraft(rows => rows!.map((r, i) => i === index ? {...r, ...patch} : r));
    setError(null);
  };
  const save = async () => {
    if (draft === null || saving.current) return;
    let targets: RepoBuildTarget[];
    try { targets = parse(draft); } catch (cause) { setError((cause as Error).message); return; }
    saving.current = true;
    setBusy(true);
    setError(null);
    try {
      onUpdated(await api.editRepo(repo.id, {build_targets: targets}));
      setDraft(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally { saving.current = false; setBusy(false); }
  };
  const targets = repo.build_targets ?? [];
  return <div className="repo-build-targets">
    {draft === null ? <>
      <span className="small">{targets.length === 0 ? "No build-host verification configured" : targets.map(t => `${t.os} / ${t.preset}${t.capabilities.length ? ` (${t.capabilities.join(", ")})` : ""}`).join("; ")}</span>{" "}
      <button type="button" className="button button--secondary" onClick={() => {setDraft(fromTargets(targets)); setError(null);}}>Edit build targets</button>
    </> : <fieldset disabled={busy}>
      <legend>Build targets for {repo.full_name}</legend>
      {draft.length === 0 && <p className="muted small">No build-host verification configured.</p>}
      {draft.map((t, i) => <div key={i} className="repo-build-targets__row">
        <label>OS <select className="input" aria-label={`Target ${i + 1} OS`} value={t.os} onChange={e => update(i, {os: e.target.value as BuildHostOS})}>
          <option value="windows">Windows</option><option value="linux">Linux</option><option value="macos">macOS</option>
        </select></label>
        <label>Preset <select className="input" aria-label={`Target ${i + 1} preset`} value={t.preset} onChange={e => update(i, {preset: e.target.value as Draft["preset"]})}>
          <option value="dotnet">dotnet</option><option value="cmake">cmake</option><option value="script">script</option>
        </select></label>
        <label>Capabilities <input className="input" aria-label={`Target ${i + 1} capabilities`} value={t.capabilities} placeholder="msvc, gl" onChange={e => update(i, {capabilities: e.target.value})}/></label>
        <button type="button" className="button button--secondary" onClick={() => {setDraft(draft.filter((_, n) => i !== n)); setError(null);}}>Remove target {i + 1}</button>
      </div>)}
      <p className="muted small">Separate capability tokens with commas or spaces.</p>
      {error && <p role="alert" className="field-error">{error}</p>}
      <button type="button" className="button button--secondary" disabled={draft.length >= 32} onClick={() => setDraft([...draft, {os: "linux", preset: "cmake", capabilities: ""}])}>Add target</button>{" "}
      <button type="button" className="button" onClick={() => void save()}>Save</button>{" "}
      <button type="button" className="button button--secondary" onClick={() => {setDraft(null); setError(null);}}>Cancel</button>
    </fieldset>}
  </div>;
}
