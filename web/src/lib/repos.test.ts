import { describe, expect, it } from "vitest";
import type { Repo } from "../api/types";
import { boardQuery } from "../api/boardApi";
import { decodeRepoFilter, encodeRepoFilter, repoColor, searchRepos } from "./repos";

function repo(id: number, fullName: string, overrides: Partial<Repo> = {}): Repo {
  return {
    id, provider: "github", full_name: fullName, name: fullName.split("/").pop() ?? fullName,
    url: "", description: "", color: "", default_branch: "main",
    is_private: false, is_archived: false, synced_at: null, ...overrides,
  };
}

describe("repo filter encoding", () => {
  it("round-trips every filter kind", () => {
    for (const filter of [{ kind: "all" }, { kind: "none" }, { kind: "repo", id: 7 }] as const) {
      expect(decodeRepoFilter(encodeRepoFilter(filter))).toEqual(filter);
    }
  });
  it("treats junk as all", () => {
    expect(decodeRepoFilter("abc")).toEqual({ kind: "all" });
    expect(decodeRepoFilter("-3")).toEqual({ kind: "all" });
  });
  it("builds the board query", () => {
    expect(boardQuery({ kind: "all" })).toBe("");
    expect(boardQuery({ kind: "none" })).toBe("?repo=none");
    expect(boardQuery({ kind: "repo", id: 4 })).toBe("?repo=4");
  });
});

describe("repoColor", () => {
  it("uses the stored color", () => {
    expect(repoColor({ color: "#123456", full_name: "a/b" })).toBe("#123456");
  });
  it("derives a stable hue when unset", () => {
    const first = repoColor({ color: "", full_name: "acme/api" });
    expect(first).toMatch(/^hsl\(\d+ /);
    expect(repoColor({ color: "", full_name: "acme/api" })).toBe(first);
  });
});

describe("searchRepos", () => {
  const repos = [
    repo(1, "acme/web"),
    repo(2, "acme/api"),
    repo(3, "other/api-tools"),
    repo(4, "acme/legacy", { is_archived: true }),
  ];
  it("hides archived repos without a query", () => {
    expect(searchRepos(repos, "").map((r) => r.id)).toEqual([1, 2, 3]);
  });
  it("matches case-insensitively and ranks name prefixes first", () => {
    expect(searchRepos(repos, "API").map((r) => r.id)).toEqual([2, 3]);
    expect(searchRepos([repo(5, "zzz/x-api"), repo(6, "aaa/api")], "api").map((r) => r.id)).toEqual([6, 5]);
  });
  it("hides archived repos on partial matches", () => {
    expect(searchRepos(repos, "legacy").map((r) => r.id)).toEqual([]);
  });
  it("shows an archived repo when named exactly", () => {
    expect(searchRepos(repos, "acme/legacy").map((r) => r.id)).toEqual([4]);
  });
});
