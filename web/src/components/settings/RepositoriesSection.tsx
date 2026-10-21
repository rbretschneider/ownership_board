import { RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Repo, RepoSource } from "../../api/types";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { Spinner } from "../common/Spinner";
import { useToast } from "../toast/ToastContext";
import { ManualRepoForm } from "./ManualRepoForm";
import { RepoList } from "./RepoList";
import { RepoSourceList } from "./RepoSourceList";

/** Repository sources (sync status) plus the synced repo list. */
export function RepositoriesSection({ onChanged }: { onChanged: () => void }) {
  const api = useApi();
  const toast = useToast();
  const [sources, setSources] = useState<RepoSource[] | null>(null);
  const [repos, setRepos] = useState<Repo[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isSyncing, setSyncing] = useState(false);

  const load = useCallback(async () => {
    setError(null);
    try {
      const [nextSources, nextRepos] = await Promise.all([api.repoSources(), api.repos()]);
      setSources(nextSources);
      setRepos(nextRepos);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [api]);

  useEffect(() => {
    void load();
  }, [load]);

  const sync = async () => {
    setSyncing(true);
    try {
      const result = await api.syncRepoSources();
      setSources(result);
      setRepos(await api.repos());
      reportSync(result, toast);
      onChanged();
    } catch (cause) {
      toast.error(`Sync failed: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setSyncing(false);
    }
  };

  const replaceRepo = (repo: Repo) => {
    setRepos((current) => (current ?? []).map((r) => (r.id === repo.id ? repo : r)));
    onChanged();
  };

  const isConfigured = (sources ?? []).some((s) => s.is_configured);

  return (
    <section className="settings-section" aria-labelledby="repos-title">
      <div className="section__head">
        <h3 id="repos-title" className="settings-section__title">Repositories</h3>
        <button type="button" className="button button--primary button--small" onClick={() => void sync()} disabled={isSyncing || !isConfigured}>
          {isSyncing ? <Spinner label="Syncing" /> : <RefreshCw size={16} aria-hidden="true" />}
          {isSyncing ? "Syncing…" : "Sync now"}
        </button>
      </div>
      {isSyncing && <p className="field__hint" role="status">Fetching repositories from each provider. This can take up to a minute.</p>}
      {error !== null && <ErrorState message={`Couldn't load repositories: ${error}`} onRetry={() => void load()} />}
      {error === null && (sources === null || repos === null) && <LoadingState label="Loading repositories" />}
      {sources !== null && <RepoSourceList sources={sources} />}
      {repos !== null && <RepoList repos={repos} onUpdated={replaceRepo} />}
      <ManualRepoForm
        onAdded={(repo) => {
          setRepos((current) => [...(current ?? []), repo].sort((a, b) => a.full_name.localeCompare(b.full_name)));
          onChanged();
        }}
      />
    </section>
  );
}

function reportSync(sources: RepoSource[], toast: { success: (m: string) => void; error: (m: string) => void }): void {
  const configured = sources.filter((s) => s.is_configured);
  for (const source of configured.filter((s) => s.last_error !== "")) {
    toast.error(`${source.label}: ${source.last_error}`);
  }
  const total = configured.reduce((sum, s) => sum + s.repo_count, 0);
  if (configured.some((s) => s.last_error === "")) {
    toast.success(`Synced ${total} ${total === 1 ? "repo" : "repos"}`);
  }
}
