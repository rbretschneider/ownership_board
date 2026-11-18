import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { QuarantinedTest, Repo } from "../../api/types";
import { ticketHash } from "../../lib/hashRoute";
import { useToast } from "../toast/ToastContext";

const STATUS_LABELS: Record<QuarantinedTest["status"], string> = {
  active: "Skipped",
  probation: "Fix merged, running again",
  released: "Released",
};

/**
 * A repo's flaky-test quarantine list. It loads when opened. Build jobs carry
 * the skipped tests; the repo's verification script decides whether to skip
 * them. A person can release a test early.
 */
export function RepoQuarantine({ repo }: { repo: Repo }) {
  const api = useApi();
  const toast = useToast();
  const [tests, setTests] = useState<QuarantinedTest[] | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const load = async () => {
    try {
      setTests((await api.quarantine(repo.id)).tests);
    } catch (cause) {
      toast.error(`Couldn't load quarantined tests for ${repo.full_name}: ${cause instanceof Error ? cause.message : String(cause)}`);
    }
  };

  const release = async (test: string) => {
    setBusy(test);
    try {
      const updated = await api.releaseQuarantine(repo.id, test);
      setTests((list) => (list ?? []).map((q) => (q.test === test ? updated : q)));
    } catch (cause) {
      toast.error(`Couldn't release "${test}": ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setBusy(null);
    }
  };

  const open = tests?.filter((q) => q.status !== "released") ?? [];
  return (
    <details className="repo-quarantine" onToggle={(e) => { if ((e.currentTarget as HTMLDetailsElement).open && tests === null) void load(); }}>
      <summary>Quarantined tests{tests !== null ? ` (${open.length})` : ""}</summary>
      {tests === null ? (
        <p className="muted small">Loading…</p>
      ) : tests.length === 0 ? (
        <p className="muted small">None. A test lands here when it fails on unrelated tickets and then passes on the same commit.</p>
      ) : (
        <ul className="repo-quarantine__list">
          {tests.map((q) => (
            <li key={q.test} className={`repo-quarantine__item repo-quarantine__item--${q.status}`}>
              <code className="repo-quarantine__name">{q.test}</code>
              <span className="badge">{STATUS_LABELS[q.status]}</span>
              <span className="muted small">
                {q.source === "seeded" ? `Seeded by ${q.added_by}` : "Detected by the board"}
                {q.status === "probation" ? ` · ${q.passes} passing run${q.passes === 1 ? "" : "s"}` : ""}
                {" · "}{q.reason}
              </span>
              {q.fix_ticket_id !== null && <a href={ticketHash(q.fix_ticket_id)}>Fix #{q.fix_ticket_id}</a>}
              {q.status !== "released" && (
                <button type="button" className="button button--secondary button--small" disabled={busy !== null} onClick={() => void release(q.test)}>
                  Release
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </details>
  );
}
