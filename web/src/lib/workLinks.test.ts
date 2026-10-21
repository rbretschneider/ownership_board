import { describe, expect, it } from "vitest";
import type { WorkLink } from "../api/types";
import { linkHost, shortSha, toWorkLinkInput, urlPathText } from "./workLinks";

describe("work link display", () => {
  it("shortens a sha to seven characters", () => {
    expect(shortSha("a1b2c3d4e5f6")).toBe("a1b2c3d");
  });
  it("shows a PR's path as its text", () => {
    expect(urlPathText("https://github.com/acme/api/pull/7/")).toBe("/acme/api/pull/7");
    expect(urlPathText("https://git.example.com/")).toBe("git.example.com");
  });
  it("prefers the server's host and falls back to parsing the URL", () => {
    const link = { host: "", url: "https://GitHub.com/acme/api/pull/7" } as WorkLink;
    expect(linkHost(link)).toBe("github.com");
    expect(linkHost({ ...link, host: "git.example.com" })).toBe("git.example.com");
  });
});

describe("toWorkLinkInput", () => {
  it("requires a full web address for a PR", () => {
    expect(toWorkLinkInput({ kind: "pr", url: "javascript:alert(1)", sha: "" })).toHaveProperty("problem");
    expect(toWorkLinkInput({ kind: "pr", url: "https://u:p@github.com/x", sha: "" })).toHaveProperty("problem");
    expect(toWorkLinkInput({ kind: "pr", url: " https://github.com/acme/api/pull/7 ", sha: "" })).toEqual({
      input: { kind: "pr", url: "https://github.com/acme/api/pull/7" },
    });
  });
  it("validates and lowercases a commit sha, with an optional URL", () => {
    expect(toWorkLinkInput({ kind: "commit", url: "", sha: "ABCDEF1" })).toEqual({ input: { kind: "commit", sha: "abcdef1" } });
    expect(toWorkLinkInput({ kind: "commit", url: "", sha: "xyz" })).toHaveProperty("problem");
    expect(toWorkLinkInput({ kind: "commit", url: "ftp://x", sha: "abcdef1" })).toHaveProperty("problem");
  });
});
