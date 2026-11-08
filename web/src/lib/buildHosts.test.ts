import { expect, it } from "vitest";
import { installCommands, shellLiteral, powershellLiteral, formatBytes, factBoolean } from "./buildHosts";
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
