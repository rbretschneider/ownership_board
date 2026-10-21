import { CircleAlert, CircleCheck, RefreshCw, Unplug } from "lucide-react";
import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Connection } from "../../api/types";
import { describeAccount, describeSync, isNotInstalledError } from "../../lib/connections";
import { Spinner } from "../common/Spinner";
import { ProviderGlyph } from "../repos/ProviderGlyph";
import { BITBUCKET_FORM, FORGEJO_FORM } from "./providerForms";
import { ProviderTokenForm } from "./ProviderTokenForm";

/** Props for {@link ConnectionCard}. */
export interface ConnectionCardProps {
  connection: Connection;
  isSyncing: boolean;
  /** Another card's sync is running, so this one waits. */
  isBusy: boolean;
  onSync: () => void;
  onDisconnect: () => void;
  onConnectGitHub: () => void;
  onConnected: (connection: Connection) => void;
}

/** One connected (or server-configured) provider with its sync state. */
export function ConnectionCard(props: ConnectionCardProps) {
  const { connection } = props;
  const api = useApi();
  const [isReconnecting, setReconnecting] = useState(false);
  const isServerManaged = connection.managed_by === "server_env";
  const needsReconnect = connection.status === "needs_reconnect";
  const syncError = connection.last_sync?.error ?? "";
  const spec = connection.provider === "forgejo" ? FORGEJO_FORM : connection.provider === "bitbucket" ? BITBUCKET_FORM : null;

  const reconnect = () => {
    if (connection.provider === "github") {
      props.onConnectGitHub();
      return;
    }
    setReconnecting(true);
  };

  return (
    <li className={`connection${needsReconnect ? " connection--warning" : ""}`}>
      <div className="connection__head">
        <ProviderGlyph provider={connection.provider} />
        <div className="connection__text">
          <span className="connection__label">{connection.label}</span>
          <span className="connection__account">
            {isServerManaged ? "Set on the server" : describeAccount(connection)}
          </span>
          <span className="muted small">{describeSync(connection)}</span>
        </div>
        {statusIcon(needsReconnect, syncError)}
      </div>
      {needsReconnect && (
        <p className="connection__problem" role="alert">
          The board can no longer use this connection. Reconnect to keep repos in sync.
        </p>
      )}
      {!needsReconnect && syncError !== "" && isNotInstalledError(syncError) && (
        <div className="connection__problem connection__problem--prominent" role="alert">
          <strong>The GitHub App isn't installed yet, so GitHub shows the board no repos.</strong>
          <p>{syncError}</p>
        </div>
      )}
      {!needsReconnect && syncError !== "" && !isNotInstalledError(syncError) && (
        <p className="connection__problem">Last sync failed: {syncError}</p>
      )}
      {isServerManaged && <p className="field__hint">Configured with environment variables on the server; change it there.</p>}
      <div className="row-actions connection__actions">
        {!isServerManaged && (
          <button type="button" className="button button--ghost button--small" onClick={props.onDisconnect} disabled={props.isSyncing}>
            <Unplug size={14} aria-hidden="true" />
            Disconnect
          </button>
        )}
        {needsReconnect ? (
          <button type="button" className="button button--primary button--small" onClick={reconnect}>Reconnect</button>
        ) : (
          <button type="button" className="button button--primary button--small" onClick={props.onSync} disabled={props.isSyncing || props.isBusy}>
            {props.isSyncing ? <Spinner label="Syncing" /> : <RefreshCw size={14} aria-hidden="true" />}
            {props.isSyncing ? "Syncing…" : syncError !== "" ? "Retry sync" : "Sync now"}
          </button>
        )}
      </div>
      {isReconnecting && spec !== null && (
        <ProviderTokenForm
          idPrefix={`reconnect-${connection.provider}`}
          title={`Reconnect ${spec.title}`}
          field={spec.field}
          tokenHint={spec.tokenHint}
          initialAccount={connection.provider === "forgejo" ? connection.base_url : connection.email}
          submitLabel="Reconnect"
          onSubmit={(account, token) => spec.submit(api, account, token)}
          onConnected={(updated) => {
            setReconnecting(false);
            props.onConnected(updated);
          }}
          onCancel={() => setReconnecting(false)}
        />
      )}
    </li>
  );
}

function statusIcon(needsReconnect: boolean, syncError: string) {
  if (needsReconnect || syncError !== "") {
    return <CircleAlert size={18} className="source__icon source__icon--error" aria-label="Needs attention" />;
  }
  return <CircleCheck size={18} className="source__icon source__icon--ok" aria-label="Working" />;
}
