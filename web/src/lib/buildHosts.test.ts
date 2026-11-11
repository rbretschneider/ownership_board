import { expect, it } from "vitest";
import type { BuildHost } from "../api/types";
import {
  describeHostSummary,
  describePresence,
  factBoolean,
  formatBytes,
  installCommands,
  powershellLiteral,
  shellLiteral,
} from "./buildHosts";
it("quotes shell literals without evaluating host configuration", () => {
  expect(shellLiteral("a'b$()`")).toBe("'a'\"'\"'b$()`'");
  expect(powershellLiteral("a'b$()")).toBe("'a''b$()'");
});
it("uses the configured base for three downloads and passes credentials only as arguments", () => {
  const commands = installCommands("https://board.example/base/", "ABC-123");
  expect(commands).toHaveLength(3);
  for (const { command } of commands) {
    expect(command).toContain("https://board.example/base/install/build-host.");
    expect(command).not.toContain("?code");
    expect(command).toContain("ABC-123");
  }
  expect(commands[0]!.command).not.toContain("GetTempFileName()");
  expect(commands[0]!.command).toContain("-PairingCode 'ABC-123'");
  expect(commands[1]!.command).toContain("--pairing-code 'ABC-123'");
  expect(installCommands("", "code")).toEqual([]);
});
it("distinguishes unknown facts from explicit zero and false", () => {
  expect(formatBytes(undefined)).toBe("Unknown");
  expect(formatBytes(0)).toBe("0 B");
  expect(factBoolean(false)).toBe("No");
  expect(factBoolean(undefined)).toBe("Unknown");
});
it("describes presence relative to now and never-seen hosts", () => {
  const now = new Date("2026-01-01T12:00:00Z");
  expect(describePresence({ online: true, last_seen_at: "2026-01-01T11:59:50Z" }, now)).toBe("Online · seen just now");
  expect(describePresence({ online: false, last_seen_at: "2026-01-01T09:00:00Z" }, now)).toBe("Offline · last seen 3h ago");
  expect(describePresence({ online: false, last_seen_at: null }, now)).toBe("Offline · never seen");
});
it("summarizes agent, build disk and capacity, falling back to the reported build volume", () => {
  const host: BuildHost = {
    id: 1,
    name: "h",
    os: "linux",
    arch: "amd64",
    status: "approved",
    capabilities: [],
    max_parallel: 2,
    enabled: true,
    online: true,
    last_seen_at: null,
    agent_version: "1.0",
    facts: { disks: [{ mount: "/", total: 10, free: 1024, is_build_volume: true }] },
  };
  expect(describeHostSummary(host)).toBe("Agent 1.0 · 1.0 KiB free · 2 builds at a time");
  expect(describeHostSummary({ ...host, status: "pending", agent_version: "", facts: {} })).toBe("Agent unknown");
});
