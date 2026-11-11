import { useEffect, useRef, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BuildHostPairingCode } from "../../api/types";
import { installCommands } from "../../lib/buildHosts";
import { Overlay } from "../common/Overlay";
import { CopyButton } from "../common/CopyButton";
import { LoadingState } from "../common/LoadingState";

export function BuildHostPairingDialog({
  issue,
  onClose,
}: {
  issue: Promise<BuildHostPairingCode>;
  onClose: () => void;
}) {
  const api = useApi();
  const [code, setCode] = useState<BuildHostPairingCode | null>(null),
    [error, setError] = useState<string | null>(null),
    [busy, setBusy] = useState(true),
    [now, setNow] = useState(Date.now());
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    let current = true;
    void issue.then(
      (value) => {
        if (current) {
          setNow(Date.now());
          setCode(value);
          setBusy(false);
        }
      },
      (cause) => {
        if (current) {
          setError(String(cause));
          setBusy(false);
        }
      },
    );
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => {
      current = false;
      active.current = false;
      window.clearInterval(timer);
    };
  }, [issue]);
  const replace = async () => {
    if (busy) return;
    setCode(null);
    setBusy(true);
    setError(null);
    try {
      const next = await api.issueBuildHostCode();
      if (active.current) {
        setNow(Date.now());
        setCode(next);
      }
    } catch (cause) {
      if (active.current) setError(String(cause));
    } finally {
      if (active.current) setBusy(false);
    }
  };
  const seconds = code ? Math.max(0, Math.ceil((Date.parse(code.expires_at) - now) / 1000)) : 0;
  return (
    <Overlay variant="dialog" label="Add build host" onClose={onClose}>
      <div className="sheet build-host-pairing">
        <header className="sheet__header">
          <h2 className="sheet__title">Add build host</h2>
        </header>
        <div className="sheet__body">
          <p className="muted">
            Run the installer for its OS on the machine you want to add. It pairs with this code, then waits under Build hosts
            for your approval.
          </p>
          {busy && <LoadingState label="Creating pairing code" />}
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          {code && (
            <>
              <div className="build-host-pairing__code">
                <div className="build-host-pairing__code-text">
                  <span className="field__label field__label--small">Pairing code</span>
                  <strong className="build-host-pairing__value">{code.code}</strong>
                  <span role="status" className={seconds > 0 ? "muted small" : "field__hint field__hint--error"}>
                    {seconds > 0
                      ? `Expires in ${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`
                      : "Pairing code expired"}
                  </span>
                </div>
                {seconds > 0 && <CopyButton text={code.code} label="Copy pairing code" />}
              </div>
              {!code.public_base_url && (
                <p className="form-error" role="alert">
                  Configure PUBLIC_BASE_URL to create installation commands.
                </p>
              )}
              {installCommands(code.public_base_url, code.code).map(({ label, command }) => (
                <div key={label} className="snippet">
                  <div className="snippet__head">
                    <span className="field__label field__label--small">{label}</span>
                    {seconds > 0 && <CopyButton text={command} label={`Copy ${label} command`} />}
                  </div>
                  <pre className="snippet__code">
                    <code>{command}</code>
                  </pre>
                </div>
              ))}
            </>
          )}
        </div>
        <footer className="sheet__footer">
          {!busy && (seconds === 0 || error) && (
            <button type="button" className="button button--secondary" onClick={() => void replace()}>
              Create replacement code
            </button>
          )}
          <button type="button" className="button button--primary" onClick={onClose}>
            Close
          </button>
        </footer>
      </div>
    </Overlay>
  );
}
