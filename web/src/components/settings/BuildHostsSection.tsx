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
  const rows = (values: BuildHost[]) =>
    values.map((h) => <BuildHostRow key={h.id} host={h} onMutate={mutate} onRevoke={() => setRevoke(h)} />);
  return (
    <section id="build-hosts" tabIndex={-1} className="settings-section build-hosts" aria-label="Build hosts">
      <h2 className="settings-section__title">Build hosts</h2>
      <button
        type="button"
        className="button"
        disabled={issue !== null}
        onClick={() => {
          if (pairingOpen.current) return;
          pairingOpen.current = true;
          setIssue(api.issueBuildHostCode());
        }}
      >
        Add build host
      </button>
      {hosts.error && <ErrorState message={hosts.error} onRetry={hosts.refresh} />}
      {hosts.value === null && !hosts.error && <LoadingState label="Loading build hosts" />}
      {hosts.value?.length === 0 && <p>No build hosts registered.</p>}
      <h3>Waiting for approval</h3>
      {pending.length === 0 ? <p>No hosts awaiting approval.</p> : rows(pending)}
      <h3>Hosts</h3>
      {approved.length === 0 ? <p>No approved hosts.</p> : rows(approved)}
      {revoked.length > 0 && (
        <details>
          <summary>Revoked ({revoked.length})</summary>
          {rows(revoked)}
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
          title="Revoke build host"
          message={`Revoke ${revoke.name}? This prevents further access; it does not stop an operating-system process immediately.`}
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
