import { ArrowLeft } from "lucide-react";
import { useEffect, useRef } from "react";
import type { AuthStatus } from "../../api/types";
import { BOARD_HASH, navigate, SETTINGS_HASH, type Route } from "../../lib/hashRoute";
import { Overlay } from "../common/Overlay";
import { useToast } from "../toast/ToastContext";
import { AccountSection } from "./AccountSection";
import { AlertsSection } from "./AlertsSection";
import { AppearanceSection } from "./AppearanceSection";
import { RepositoriesSection } from "./RepositoriesSection";
import { TokensSection } from "./TokensSection";

/** Props for {@link SettingsSheet}. */
export interface SettingsSheetProps {
  route: Route;
  status: AuthStatus;
  onSignedOut: () => void;
  /** Called after repos or the account change, so dependents refetch. */
  onChanged: () => void;
}

/** Settings (#/settings): repositories, agent tokens, appearance, account. */
export function SettingsSheet({ route, status, onSignedOut, onChanged }: SettingsSheetProps) {
  const toast = useToast();
  const isJustLinked = route.params.get("linked") === "1";
  const hasReportedLink = useRef(false);

  // The OIDC link flow lands on #/settings?linked=1: confirm once, then tidy the URL.
  useEffect(() => {
    if (isJustLinked && !hasReportedLink.current) {
      hasReportedLink.current = true;
      toast.success(`${status.oidc.label} linked — you can now sign in with it.`);
      window.history.replaceState(null, "", SETTINGS_HASH);
      onChanged();
    }
  }, [isJustLinked, toast, status.oidc.label, onChanged]);

  const close = () => navigate(BOARD_HASH);

  return (
    <Overlay variant="panel" label="Settings" onClose={close}>
      <div className="sheet settings">
        <header className="sheet__header">
          <button type="button" className="icon-button" aria-label="Back to board" onClick={close}>
            <ArrowLeft size={20} aria-hidden="true" />
          </button>
          <h2 className="sheet__title">Settings</h2>
        </header>
        <div className="sheet__body settings__body">
          <RepositoriesSection onChanged={onChanged} />
          <TokensSection />
          <AppearanceSection />
          <AlertsSection />
          <AccountSection status={status} onSignedOut={onSignedOut} />
        </div>
      </div>
    </Overlay>
  );
}
