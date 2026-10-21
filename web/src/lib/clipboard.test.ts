import { describe, expect, it, vi } from "vitest";
import { copyText, type ClipboardEnvironment } from "./clipboard";

function environment(overrides: Partial<ClipboardEnvironment> = {}): ClipboardEnvironment {
  return { isSecureContext: false, clipboard: undefined, document, ...overrides };
}

describe("copyText", () => {
  it("uses the Clipboard API on a secure origin", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const ok = await copyText("secret", environment({ isSecureContext: true, clipboard: { writeText } }));
    expect(ok).toBe(true);
    expect(writeText).toHaveBeenCalledWith("secret");
  });

  it("falls back to the copy command over plain HTTP", async () => {
    const execCommand = vi.fn().mockReturnValue(true);
    document.execCommand = execCommand;
    const ok = await copyText("ob_pat_x", environment());
    expect(ok).toBe(true);
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(document.querySelectorAll("textarea")).toHaveLength(0);
  });

  it("falls back when the Clipboard API refuses", async () => {
    document.execCommand = vi.fn().mockReturnValue(true);
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    expect(await copyText("x", environment({ isSecureContext: true, clipboard: { writeText } }))).toBe(true);
  });

  it("reports failure when nothing can copy", async () => {
    document.execCommand = vi.fn().mockReturnValue(false);
    expect(await copyText("x", environment())).toBe(false);
  });
});
