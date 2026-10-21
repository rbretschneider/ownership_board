import { describe, expect, it, vi } from "vitest";
import { isPlainHttp, submitSecret } from "./secretField";

describe("submitSecret", () => {
  it("clears the field before the request settles and sends the trimmed value", async () => {
    const events: string[] = [];
    const send = vi.fn((secret: string) => {
      events.push(`send:${secret}`);
      return Promise.resolve("ok");
    });
    const result = await submitSecret("  tok  ", () => events.push("clear"), send);
    expect(result).toBe("ok");
    expect(events).toEqual(["clear", "send:tok"]);
  });

  it("still clears when the request fails", async () => {
    const clear = vi.fn();
    await expect(submitSecret("tok", clear, () => Promise.reject(new Error("rejected")))).rejects.toThrow("rejected");
    expect(clear).toHaveBeenCalledOnce();
  });
});

describe("isPlainHttp", () => {
  it("flags http but not https", () => {
    expect(isPlainHttp("http://10.0.0.5:3000")).toBe(true);
    expect(isPlainHttp(" HTTP://git.example.com")).toBe(true);
    expect(isPlainHttp("https://git.example.com")).toBe(false);
    expect(isPlainHttp("")).toBe(false);
  });
});
