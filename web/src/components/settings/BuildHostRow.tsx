import { Ban, Info, Pencil, Server } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useApi } from "../../api/ApiContext";
import type { BuildHost } from "../../api/types";
import { describeHostSummary, describePresence, OS_LABELS } from "../../lib/buildHosts";
import { relativeTime } from "../../lib/relativeTime";
import { RowMenu, type RowMenuItem } from "../common/RowMenu";
import { BuildHostCapabilities } from "./BuildHostCapabilities";
import { BuildHostEditor } from "./BuildHostEditor";
import { BuildHostEvents } from "./BuildHostEvents";
import { BuildHostFacts } from "./BuildHostFacts";

/** Props for {@link BuildHostRow}. */
export interface BuildHostRowProps {
  host: BuildHost;
  onMutate: (id: number, operation: () => Promise<BuildHost>) => Promise<void>;
  /** Asks the owner to confirm revoking (or rejecting) this host. */
  onRevoke: () => void;
}

/** One build host as a card: approved hosts collapse their configuration behind Edit. */
export function BuildHostRow({ host, onMutate, onRevoke }: BuildHostRowProps) {
  const [isBusy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  const run = async (operation: () => Promise<BuildHost>): Promise<boolean> => {
    if (isBusy) {
      return false;
    }
    setBusy(true);
    setError(null);
    try {
      await onMutate(host.id, operation);
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      return false;
    } finally {
      setBusy(false);
    }
  };
  if (host.status === "revoked") {
    return <RevokedHost host={host} />;
  }
  if (host.status === "pending") {
    return <PendingHost host={host} isBusy={isBusy} error={error} run={run} onReject={onRevoke} />;
  }
  return <ApprovedHost host={host} isBusy={isBusy} error={error} run={run} onRevoke={onRevoke} clearError={() => setError(null)} />;
}

type Run = (operation: () => Promise<BuildHost>) => Promise<boolean>;

function HostHeading({ host, children }: { host: BuildHost; children?: ReactNode }) {
  return (
    <div className="connection__head build-host__head">
      <Server size={18} className="build-host__glyph" aria-hidden="true" />
      <div className="connection__text">
        <span className="build-host__title">
          <span className="connection__label">{host.name}</span>
          <span className="badge">{OS_LABELS[host.os]}</span>
          {host.status === "approved" && !host.enabled && <span className="badge badge--pr-idle">Paused</span>}
        </span>
        {children}
      </div>
    </div>
  );
}

function Presence({ host }: { host: BuildHost }) {
  return (
    <span className="build-host__presence">
      <span className={`build-host__dot${host.online ? " build-host__dot--online" : ""}`} aria-hidden="true" />
      <span title={host.last_seen_at ? new Date(host.last_seen_at).toLocaleString() : undefined}>{describePresence(host)}</span>
    </span>
  );
}

function CapabilityBadges({ values }: { values: string[] }) {
  if (values.length === 0) {
    return <span className="muted small">No capabilities</span>;
  }
  return (
    <ul className="build-host__capabilities" aria-label="Capabilities">
      {values.map((value) => (
        <li key={value} className="badge">
          {value}
        </li>
      ))}
    </ul>
  );
}

function ApprovedHost(props: { host: BuildHost; isBusy: boolean; error: string | null; run: Run; onRevoke: () => void; clearError: () => void }) {
  const { host, isBusy, error, run } = props;
  const api = useApi();
  const [isEditing, setEditing] = useState(false),
    [isExpanded, setExpanded] = useState(false);
  const menu: RowMenuItem[] = [
    { label: isExpanded ? "Hide details" : "Show details", icon: Info, onSelect: () => setExpanded((v) => !v) },
    { label: "Revoke…", icon: Ban, onSelect: props.onRevoke, isDanger: true },
  ];
  const closeEditor = () => {
    props.clearError();
    setEditing(false);
  };
  return (
    <li className="connection build-host" aria-label={host.name}>
      <div className="build-host__top">
        <HostHeading host={host}>
          <Presence host={host} />
          <span className="muted small">{describeHostSummary(host)}</span>
          {!isEditing && <CapabilityBadges values={host.capabilities} />}
        </HostHeading>
        <div className="build-host__actions">
          {!isEditing && (
            <button type="button" className="button button--secondary button--small" onClick={() => setEditing(true)}>
              <Pencil size={14} aria-hidden="true" />
              Edit
            </button>
          )}
          <RowMenu label={host.name} items={menu} />
        </div>
      </div>
      {isEditing && (
        <BuildHostEditor
          host={host}
          isBusy={isBusy}
          error={error}
          onCancel={closeEditor}
          onSave={async (edit) => {
            const isSaved = await run(() => api.editBuildHost(host.id, edit));
            if (isSaved) {
              setEditing(false);
            }
            return isSaved;
          }}
        />
      )}
      {!isEditing && error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {isExpanded && (
        <div className="build-host__details">
          <BuildHostFacts host={host} />
          <BuildHostEvents id={host.id} />
        </div>
      )}
    </li>
  );
}

function PendingHost(props: { host: BuildHost; isBusy: boolean; error: string | null; run: Run; onReject: () => void }) {
  const { host, isBusy, run } = props;
  const api = useApi();
  const [capabilities, setCapabilities] = useState(host.capabilities);
  const isDirty = useRef(false);
  useEffect(() => {
    if (!isDirty.current) {
      setCapabilities(host.capabilities);
    }
  }, [host]);
  return (
    <li className="connection connection--warning build-host build-host--pending" aria-label={host.name}>
      <HostHeading host={host}>
        <Presence host={host} />
        <span className="muted small">{describeHostSummary(host)}</span>
      </HostHeading>
      <BuildHostFacts host={host} />
      <BuildHostCapabilities
        values={capabilities}
        disabled={isBusy}
        onChange={(values) => {
          isDirty.current = true;
          setCapabilities(values);
        }}
      />
      {props.error && (
        <p className="form-error" role="alert">
          {props.error}
        </p>
      )}
      <div className="row-actions">
        <button type="button" className="button button--ghost button--small build-host__reject" disabled={isBusy} onClick={props.onReject}>
          Reject
        </button>
        <button
          type="button"
          className="button button--primary button--small"
          disabled={isBusy}
          onClick={() => void run(() => api.approveBuildHost(host.id, { capabilities }))}
        >
          Approve
        </button>
      </div>
    </li>
  );
}

function RevokedHost({ host }: { host: BuildHost }) {
  return (
    <li className="connection build-host build-host--revoked" aria-label={host.name}>
      <HostHeading host={host}>
        <span className="muted small">Revoked {host.revoked_at ? relativeTime(host.revoked_at) : ""}</span>
      </HostHeading>
    </li>
  );
}
