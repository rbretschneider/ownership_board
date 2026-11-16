import type { RunnerName } from "../api/types";

/** Assigned runner names by worker id ("claude_1"). */
export type RunnerDirectory = ReadonlyMap<string, RunnerName>;

/** No names known yet: every runner shows its raw identity. */
export const EMPTY_RUNNER_DIRECTORY: RunnerDirectory = new Map();

const WORKER_IDENTITY = /^@worker\/\d+\/([A-Za-z0-9_-]+)$/;

/** How a runner reads in the UI. */
export interface RunnerLabel {
  /** The name alone, "Michael", or the worker id ("claude_1") without a name. */
  label: string;
  /** What the UI shows: "Michael (claude_1)", or the label alone without a name. */
  detail: string;
  /** For tooltips: the raw identity, which leases and timelines match on. */
  title: string;
  /** The family to badge the name with when another family has a runner of the same name; "" otherwise. */
  badge: string;
}

/** Indexes assigned names by worker id. */
export function runnerDirectory(names: readonly RunnerName[]): RunnerDirectory {
  return new Map(names.map((name) => [name.worker, name]));
}

/** "@worker/7/codex_1" is worker "codex_1"; any other name is not a worker. */
export function workerIdOf(identity: string): string | undefined {
  return WORKER_IDENTITY.exec(identity)?.[1];
}

/**
 * How to show a runner identity. A runner without a name (an unpooled family,
 * or a plain agent token) falls back to its worker id or the identity itself.
 * known is a name a payload already carries (a Team runner), used until the
 * directory has loaded it.
 */
export function labelRunner(identity: string, directory: RunnerDirectory, known?: KnownRunnerName): RunnerLabel {
  const worker = workerIdOf(identity);
  const named = worker === undefined ? undefined : directory.get(worker) ?? fromKnown(worker, known);
  if (worker === undefined || named === undefined || named.display_name === "") {
    const label = worker ?? identity;
    return { label, detail: label, title: identity, badge: "" };
  }
  const label = named.display_name;
  return { label, detail: `${label} (${worker})`, title: identity, badge: isNameShared(named, directory) ? named.family : "" };
}

/**
 * Names the runners an agent wrote about in free text (a manager summary):
 * "@worker/7/claude_1" or a bare "claude_1" reads "Michael (claude_1)".
 * Mentions already followed by a name in parentheses stay as they are.
 */
export function nameRunnersInText(text: string, directory: RunnerDirectory): string {
  if (directory.size === 0) return text;
  return text.replace(/(\()?(?:@worker\/\d+\/)?\b([A-Za-z]+[_-]\d+)\b/g, (match, paren: string | undefined, worker: string) => {
    const named = directory.get(worker);
    if (paren !== undefined || named === undefined || named.display_name === "") return match;
    return `${named.display_name} (${worker})`;
  });
}

/** Whether a runner of another family shares the name, so the family must tell them apart. */
function isNameShared(name: RunnerName, directory: RunnerDirectory): boolean {
  const key = name.display_name.toLowerCase();
  for (const other of directory.values()) {
    if (other.family !== name.family && other.display_name.toLowerCase() === key) return true;
  }
  return false;
}

/** A display name a payload carries alongside the identity. */
export interface KnownRunnerName {
  display_name?: string;
  family?: string;
}

function fromKnown(worker: string, known: KnownRunnerName | undefined): RunnerName | undefined {
  if (known?.display_name === undefined || known.display_name === "") return undefined;
  return { worker, family: known.family ?? "", position: 0, display_name: known.display_name, assigned_at: null };
}
