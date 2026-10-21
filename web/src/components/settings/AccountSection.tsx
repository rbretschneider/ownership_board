import { Link2, LogOut, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { AuthStatus } from "../../api/types";
import { OIDC_LINK_URL } from "../../lib/constants";
import { Spinner } from "../common/Spinner";
import { useToast } from "../toast/ToastContext";

/** Who is signed in, SSO linking, and sign out. */
export function AccountSection({ status, onSignedOut }: { status: AuthStatus; onSignedOut: () => void }) {
  const api = useApi();
  const toast = useToast();
  const [isSigningOut, setSigningOut] = useState(false);
  const user = status.user;

  const signOut = async () => {
    setSigningOut(true);
    try {
      await api.logout();
      onSignedOut();
    } catch (cause) {
      toast.error(`Couldn't sign out: ${cause instanceof Error ? cause.message : String(cause)}`);
      setSigningOut(false);
    }
  };

  return (
    <section className="settings-section" aria-labelledby="account-title">
      <h3 id="account-title" className="settings-section__title">Account</h3>
      {user !== null && (
        <p>
          Signed in as <strong>{user.name}</strong> <span className="muted">({user.email})</span>
        </p>
      )}
      {status.oidc.enabled && user !== null && (
        user.sso_linked ? (
          <p className="sso-linked">
            <ShieldCheck size={16} aria-hidden="true" />
            {status.oidc.label} linked
          </p>
        ) : (
          <a className="button button--secondary" href={OIDC_LINK_URL}>
            <Link2 size={16} aria-hidden="true" />
            Link {status.oidc.label}
          </a>
        )
      )}
      <div>
        <button type="button" className="button button--danger-subtle" onClick={() => void signOut()} disabled={isSigningOut}>
          {isSigningOut ? <Spinner label="Signing out" /> : <LogOut size={16} aria-hidden="true" />}
          Sign out
        </button>
      </div>
    </section>
  );
}
