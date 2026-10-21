import { describe, expect, it } from "vitest";
import type { Connection } from "../api/types";
import { canConnectGitHub, describeAccount, describeSync, hasCard, isNotInstalledError, needsGitHubSetup } from "./connections";

function connection(overrides: Partial<Connection> = {}): Connection {
  return {
    provider: "github", label: "GitHub", is_available: true, is_connected: false, status: "disconnected",
    managed_by: null, account: "", base_url: "", email: "", connected_at: null, repo_count: 0, last_sync: null, ...overrides,
  };
}

describe("connection cards", () => {
  it("shows cards only for connected, broken or server-managed providers", () => {
    expect(hasCard(connection())).toBe(false);
    expect(hasCard(connection({ status: "connected" }))).toBe(true);
    expect(hasCard(connection({ status: "needs_reconnect" }))).toBe(true);
    expect(hasCard(connection({ managed_by: "server_env" }))).toBe(true);
  });

  it("offers Connect GitHub only when available and not set up", () => {
    expect(canConnectGitHub([connection()])).toBe(true);
    expect(canConnectGitHub([connection({ is_available: false })])).toBe(false);
    expect(canConnectGitHub([connection({ status: "connected" })])).toBe(false);
    expect(canConnectGitHub([connection({ managed_by: "server_env" })])).toBe(false);
    expect(canConnectGitHub([])).toBe(false);
  });

  it("describes the account with what tells it apart", () => {
    expect(describeAccount(connection({ account: "octo" }))).toBe("Connected as @octo");
    expect(describeAccount(connection({ provider: "forgejo", account: "me", base_url: "http://10.0.0.5:3021" }))).toBe("Connected as @me on 10.0.0.5:3021");
    expect(describeAccount(connection({ provider: "bitbucket", account: "me", email: "me@example.com" }))).toBe("Connected as @me (me@example.com)");
  });

  it("describes the sync state", () => {
    const now = new Date("2026-01-01T10:05:00Z");
    expect(describeSync(connection({ repo_count: 1 }), now)).toBe("1 repo · never synced");
    expect(describeSync(connection({ repo_count: 4, last_sync: { at: "2026-01-01T10:00:00Z", error: "" } }), now)).toBe("4 repos · synced 5m ago");
  });
});

describe("GitHub setup and install state", () => {
  it("shows the setup card only when GitHub has no client ID and no env token", () => {
    expect(needsGitHubSetup([connection({ is_available: false })])).toBe(true);
    expect(needsGitHubSetup([connection()])).toBe(false);
    expect(needsGitHubSetup([connection({ is_available: false, managed_by: "server_env" })])).toBe(false);
  });
  it("recognises the not-installed sync error", () => {
    expect(isNotInstalledError("The GitHub App is not installed on any account")).toBe(true);
    expect(isNotInstalledError("the token was rejected (HTTP 401)")).toBe(false);
  });
});
