import { useEffect, useRef, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { BuildHostPairingCode } from "../../api/types";
import { installCommands } from "../../lib/buildHosts";
import { Overlay } from "../common/Overlay";
import { CopyButton } from "../common/CopyButton";

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
        <div className="sheet__header">
          <h2 className="sheet__title">Add build host</h2>
          <button type="button" className="button" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="sheet__body">
          {busy && <p role="status">Creating pairing code…</p>}
          {error && <p role="alert">{error}</p>}
          {code && (
            <>
              <p>
                Pairing code: <strong>{code.code}</strong>{" "}
                {seconds > 0 && <CopyButton text={code.code} label="Copy pairing code" />}
              </p>
              <p role="status">
                {seconds > 0
                  ? `Expires in ${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`
                  : "Pairing code expired"}
              </p>
              {!code.public_base_url && <p role="alert">Configure PUBLIC_BASE_URL to create installation commands.</p>}
              {installCommands(code.public_base_url, code.code).map(({ label, command }) => (
                <div key={label}>
                  <h3>{label}</h3>
                  <pre>{command}</pre>
                  {seconds > 0 && <CopyButton text={command} label={`Copy ${label} command`} />}
                </div>
              ))}
            </>
          )}
          {!busy && (seconds === 0 || error) && (
            <button type="button" className="button" onClick={() => void replace()}>
              Create replacement code
            </button>
          )}
        </div>
      </div>
    </Overlay>
  );
}
