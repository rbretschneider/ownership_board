import type { WorkLink, WorkLinkInput } from "../api/types";

/** Characters of a commit sha shown in the UI. */
export const SHORT_SHA_LENGTH = 7;

const SHA_PATTERN = /^[0-9a-f]{7,40}$/;

/** The first seven characters of a sha. */
export function shortSha(sha: string): string {
  return sha.slice(0, SHORT_SHA_LENGTH);
}

/** The host of a URL, or "" when it does not parse. */
export function urlHost(url: string): string {
  try {
    return new URL(url).host.toLowerCase();
  } catch {
    return "";
  }
}

/** Link text for a PR: the URL's path, e.g. "/acme/api/pull/7". */
export function urlPathText(url: string): string {
  try {
    const parsed = new URL(url);
    return parsed.pathname === "/" ? parsed.host : parsed.pathname.replace(/\/$/, "");
  } catch {
    return url;
  }
}

/** The host to show next to a link: the server's, falling back to parsing. */
export function linkHost(link: WorkLink): string {
  return link.host !== "" ? link.host : urlHost(link.url);
}

/** True when the URL is an absolute http(s) URL without a user name or password. */
export function isWebUrl(value: string): boolean {
  try {
    const parsed = new URL(value);
    return (parsed.protocol === "http:" || parsed.protocol === "https:") && parsed.username === "" && parsed.password === "";
  } catch {
    return false;
  }
}

/** Form values for a new work link. */
export interface WorkLinkDraft {
  kind: WorkLinkInput["kind"];
  url: string;
  sha: string;
}

/**
 * Validates a draft and builds the request body, or returns the problem to
 * show. The server re-checks everything, including the repo host.
 */
export function toWorkLinkInput(draft: WorkLinkDraft): { input: WorkLinkInput } | { problem: string } {
  const url = draft.url.trim();
  if (draft.kind === "pr") {
    return isWebUrl(url) ? { input: { kind: "pr", url } } : { problem: "Enter the pull request's full https:// address." };
  }
  const sha = draft.sha.trim().toLowerCase();
  if (!SHA_PATTERN.test(sha)) {
    return { problem: "A commit sha is 7 to 40 hex characters (0–9, a–f)." };
  }
  if (url !== "" && !isWebUrl(url)) {
    return { problem: "The commit URL must be a full https:// address, or leave it empty." };
  }
  return { input: url === "" ? { kind: "commit", sha } : { kind: "commit", sha, url } };
}
