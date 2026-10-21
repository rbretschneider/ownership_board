import type { Connection, ConnectionProvider } from "../api/types";
import { relativeTime } from "./relativeTime";

/** Whether a connection gets its own card in Settings. */
export function hasCard(connection: Connection): boolean {
  return connection.status !== "disconnected" || connection.managed_by === "server_env";
}

/** Whether the prominent "Connect GitHub" button should show. */
export function canConnectGitHub(connections: readonly Connection[]): boolean {
  const github = connections.find((c) => c.provider === "github");
  return github !== undefined && github.is_available && !hasCard(github);
}

/**
 * Whether to show the "Set up GitHub" card: the server has no client ID and
 * GitHub is not configured another way (env token).
 */
export function needsGitHubSetup(connections: readonly Connection[]): boolean {
  const github = connections.find((c) => c.provider === "github");
  return github !== undefined && !github.is_available && !hasCard(github);
}

/** Matches the server's sync error for a GitHub App with no installations. */
const NOT_INSTALLED_PATTERN = /not installed|isn't installed|no installation/i;

/** True when the last sync failed because the GitHub App is not installed anywhere. */
export function isNotInstalledError(error: string): boolean {
  return NOT_INSTALLED_PATTERN.test(error);
}

/** The connection for a provider, if the server listed it. */
export function findConnection(connections: readonly Connection[], provider: ConnectionProvider): Connection | undefined {
  return connections.find((c) => c.provider === provider);
}

/** "Connected as @login" plus the host or email that tells accounts apart. */
export function describeAccount(connection: Connection): string {
  const who = connection.account === "" ? "Connected" : `Connected as @${connection.account}`;
  if (connection.provider === "forgejo" && connection.base_url !== "") {
    return `${who} on ${hostOf(connection.base_url)}`;
  }
  if (connection.provider === "bitbucket" && connection.email !== "") {
    return `${who} (${connection.email})`;
  }
  return who;
}

/** "12 repos · synced 5m ago", or "never synced". */
export function describeSync(connection: Connection, now: Date = new Date()): string {
  const count = `${connection.repo_count} ${connection.repo_count === 1 ? "repo" : "repos"}`;
  if (connection.last_sync === null) {
    return `${count} · never synced`;
  }
  return `${count} · synced ${relativeTime(connection.last_sync.at, now)}`;
}

/** Where to revoke the board's access at the provider itself. */
export function revokeHint(provider: ConnectionProvider): string {
  switch (provider) {
    case "github":
      return "To fully revoke access, remove the app under GitHub → Settings → Applications.";
    case "forgejo":
      return "To fully revoke access, delete the token under your Forgejo user settings → Applications.";
    case "bitbucket":
      return "To fully revoke access, revoke the token in your Atlassian or Bitbucket settings.";
  }
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}
