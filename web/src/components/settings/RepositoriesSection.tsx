import { GitBranch } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Connection, ConnectionProvider, Repo } from "../../api/types";
import { canConnectGitHub, hasCard, needsGitHubSetup, revokeHint } from "../../lib/connections";
import { ConfirmDialog } from "../common/ConfirmDialog";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { useToast } from "../toast/ToastContext";
import { AddProviderPanel } from "./AddProviderPanel";
import { ConnectGitHubDialog } from "./ConnectGitHubDialog";
import { ConnectionCard } from "./ConnectionCard";
import { GitHubSetupCard } from "./GitHubSetupCard";
import { ManualRepoForm } from "./ManualRepoForm";
import { RepoList } from "./RepoList";

/**
 * Where repos come from: a Connect GitHub button or one card per connected
 * provider, a collapsed "Add another provider", then the repo list.
 */
export function RepositoriesSection({ onChanged }: { onChanged: () => void }) {
  const api = useApi();
  const toast = useToast();
  const [connections, setConnections] = useState<Connection[] | null>(null);
  const [repos, setRepos] = useState<Repo[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [syncing, setSyncing] = useState<ConnectionProvider | null>(null);
  const [isConnectingGitHub, setConnectingGitHub] = useState(false);
  const [disconnecting, setDisconnecting] = useState<Connection | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const [nextConnections, nextRepos] = await Promise.all([api.connections(), api.repos()]);
      setConnections(nextConnections);
      setRepos(nextRepos);
    } catch (cause) {
      setError(messageOf(cause));
    }
  }, [api]);

  useEffect(() => {
    void load();
  }, [load]);

  const refresh = useCallback(() => {
    void load();
    onChanged();
  }, [load, onChanged]);

  const sync = async (provider: ConnectionProvider) => {
    setSyncing(provider);
    try {
      const result = await api.syncConnections();
      setConnections(result);
      setRepos(await api.repos());
      reportSync(result.find((c) => c.provider === provider), toast);
      onChanged();
    } catch (cause) {
      toast.error(`Sync failed: ${messageOf(cause)}. Try again in a moment.`);
    } finally {
      setSyncing(null);
    }
  };

  const disconnect = async (connection: Connection) => {
    await api.disconnect(connection.provider);
    setDisconnecting(null);
    toast.success(`${connection.label} disconnected`);
    refresh();
  };

  const cards = (connections ?? []).filter(hasCard);
  const isLoading = error === null && (connections === null || repos === null);

  return (
    <section className="settings-section" aria-labelledby="repos-title">
      <h3 id="repos-title" className="settings-section__title">Repositories</h3>
      {error !== null && <ErrorState message={`Couldn't load repositories: ${error}`} onRetry={() => void load()} />}
      {isLoading && <LoadingState label="Loading repositories" />}
      {connections !== null && needsGitHubSetup(connections) && <GitHubSetupCard />}
      {connections !== null && canConnectGitHub(connections) && (
        <button type="button" className="button button--primary button--block connect-github" onClick={() => setConnectingGitHub(true)}>
          <GitBranch size={18} aria-hidden="true" />
          Connect GitHub
        </button>
      )}
      {cards.length > 0 && (
        <ul className="connection-list">
          {cards.map((connection) => (
            <ConnectionCard
              key={connection.provider}
              connection={connection}
              isSyncing={syncing === connection.provider}
              isBusy={syncing !== null}
              onSync={() => void sync(connection.provider)}
              onDisconnect={() => setDisconnecting(connection)}
              onConnectGitHub={() => setConnectingGitHub(true)}
              onConnected={(updated) => onTokenConnected(updated, toast, refresh)}
            />
          ))}
        </ul>
      )}
      {connections !== null && (
        <AddProviderPanel connections={connections} onConnected={(updated) => onTokenConnected(updated, toast, refresh)} />
      )}
      {repos !== null && <RepoList repos={repos} onUpdated={(repo) => {
        setRepos((current) => (current ?? []).map((r) => (r.id === repo.id ? repo : r)));
        onChanged();
      }} />}
      {repos !== null && (
        <ManualRepoForm
          onAdded={(repo) => {
            setRepos((current) => [...(current ?? []), repo].sort((a, b) => a.full_name.localeCompare(b.full_name)));
            onChanged();
          }}
        />
      )}
      {isConnectingGitHub && <ConnectGitHubDialog onClose={() => setConnectingGitHub(false)} onConnected={refresh} />}
      {disconnecting !== null && (
        <ConfirmDialog
          title={`Disconnect ${disconnecting.label}?`}
          message={`The board forgets its token and stops syncing. Repos already on the board stay. ${revokeHint(disconnecting.provider)}`}
          confirmLabel="Disconnect"
          onConfirm={() => disconnect(disconnecting)}
          onCancel={() => setDisconnecting(null)}
        />
      )}
    </section>
  );
}

function onTokenConnected(connection: Connection, toast: { success: (m: string) => void }, refresh: () => void): void {
  toast.success(`${connection.label} connected${connection.account !== "" ? ` as @${connection.account}` : ""} · syncing repos`);
  refresh();
}

function reportSync(connection: Connection | undefined, toast: { success: (m: string) => void; error: (m: string) => void }): void {
  if (connection === undefined) {
    return;
  }
  const error = connection.last_sync?.error ?? "";
  if (error !== "") {
    toast.error(`${connection.label}: ${error}`);
    return;
  }
  toast.success(`Synced ${connection.repo_count} ${connection.repo_count === 1 ? "repo" : "repos"}`);
}

function messageOf(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause);
}
