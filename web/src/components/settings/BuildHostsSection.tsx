import { Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BuildHost, BuildHostPairingCode } from "../../api/types";
import { useVisiblePolling } from "../../hooks/useVisiblePolling";
import { ConfirmDialog } from "../common/ConfirmDialog";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { BuildHostPairingDialog } from "./BuildHostPairingDialog";
import { BuildHostRow } from "./BuildHostRow";

export function BuildHostsSection() {
  const api = useApi();
  const load = useCallback(() => api.buildHosts(), [api]);
  const hosts = useVisiblePolling(load);
  const [issue, setIssue] = useState<Promise<BuildHostPairingCode> | null>(null),
    [revoke, setRevoke] = useState<BuildHost | null>(null);
  const active = useRef(true);
  const pairingOpen = useRef(false);
  const mutations = useRef(new Set<number>());
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const mutate = async (id: number, operation: () => Promise<BuildHost>) => {
    if (mutations.current.has(id)) throw new Error("A host update is already in progress.");
    mutations.current.add(id);
    hosts.invalidate();
    try {
      const next = await operation();
      if (active.current) {
        hosts.invalidate();
        hosts.setValue((previous) => previous?.map((h) => (h.id === id ? next : h)) ?? [next]);
      }
    } finally {
      mutations.current.delete(id);
      hosts.invalidate();
      hosts.refresh();
    }
  };
  const pending = hosts.value?.filter((h) => h.status === "pending") ?? [],
    approved = hosts.value?.filter((h) => h.status === "approved") ?? [],
    revoked = hosts.value?.filter((h) => h.status === "revoked") ?? [];
  const list = (values: BuildHost[], label: string) => (
    <ul className="connection-list" aria-label={label}>
      {values.map((h) => (
        <BuildHostRow key={h.id} host={h} onMutate={mutate} onRevoke={() => setRevoke(h)} />
      ))}
    </ul>
  );
  const openPairing = () => {
    if (pairingOpen.current) {
      return;
    }
    pairingOpen.current = true;
    setIssue(api.issueBuildHostCode());
  };
  return (
    <section id="build-hosts" tabIndex={-1} className="settings-section build-hosts" aria-labelledby="build-hosts-title">
      <div className="build-hosts__head">
        <h2 id="build-hosts-title" className="settings-section__title">
          Build hosts
        </h2>
        <button type="button" className="button button--secondary button--small" disabled={issue !== null} onClick={openPairing}>
          <Plus size={16} aria-hidden="true" />
          Add build host
        </button>
      </div>
      {hosts.error && <ErrorState message={hosts.error} onRetry={hosts.refresh} />}
      {hosts.value === null && !hosts.error && <LoadingState label="Loading build hosts" />}
      {hosts.value?.length === 0 && (
        <div className="notice">
          No build hosts yet. Add one to run builds on a machine of your own; it shows up here for approval once its installer
          pairs.
        </div>
      )}
      {pending.length > 0 && (
        <div className="build-hosts__group">
          <h3 className="field__label">Waiting for approval ({pending.length})</h3>
          {list(pending, "Waiting for approval")}
        </div>
      )}
      {approved.length > 0 && list(approved, "Approved hosts")}
      {revoked.length > 0 && (
        <details className="build-hosts__revoked">
          <summary className="muted small">Revoked ({revoked.length})</summary>
          {list(revoked, "Revoked hosts")}
        </details>
      )}
      {issue && (
        <BuildHostPairingDialog
          issue={issue}
          onClose={() => {
            pairingOpen.current = false;
            setIssue(null);
          }}
        />
      )}
      {revoke && (
        <ConfirmDialog
          title={revoke.status === "pending" ? `Reject ${revoke.name}?` : `Revoke ${revoke.name}?`}
          message={`${revoke.name} loses access to the board and takes no new builds. A build already running on it is not stopped immediately.`}
          confirmLabel="Revoke host"
          onCancel={() => setRevoke(null)}
          onConfirm={async () => {
            await mutate(revoke.id, () => api.revokeBuildHost(revoke.id));
            if (active.current) setRevoke(null);
          }}
        />
      )}
    </section>
  );
}
