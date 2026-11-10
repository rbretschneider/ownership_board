import { useEffect, useRef } from "react";
import type { AuthStatus } from "../../api/types";
import { SETTINGS_HASH, type Route } from "../../lib/hashRoute";
import { PageShell } from "../common/PageShell";
import { useToast } from "../toast/ToastContext";
import { AccountSection } from "./AccountSection";
import { BuildHostsSection } from "./BuildHostsSection";
import { AgentWorkSection } from "./AgentWorkSection";
import { ReviewSection } from "./ReviewSection";
import { AlertsSection } from "./AlertsSection";
import { AppearanceSection } from "./AppearanceSection";
import { RepositoriesSection } from "./RepositoriesSection";
import { TokensSection } from "./TokensSection";

/** Props for {@link SettingsPage}. */
export interface SettingsPageProps {
  route: Route;
  status: AuthStatus;
  onSignedOut: () => void;
  /** Called after repos or the account change, so dependents refetch. */
  onChanged: () => void;
}

/** Settings (#/settings): agent work, repositories, agent tokens, appearance, alerts, account. */
export function SettingsPage({ route, status, onSignedOut, onChanged }: SettingsPageProps) {
  const toast = useToast();
  const isJustLinked = route.params.get("linked") === "1";
  const hasReportedLink = useRef(false);

  const section = route.params.get("section");
  useEffect(() => {
    if (section !== "build-hosts") return;
    const target = document.getElementById("build-hosts");
    target?.scrollIntoView({block: "start"});
    target?.focus({preventScroll: true});
  }, [section]);

  // The OIDC link flow lands on #/settings?linked=1: confirm once, then tidy the URL.
  useEffect(() => {
    if (isJustLinked && !hasReportedLink.current) {
      hasReportedLink.current = true;
      toast.success(`${status.oidc.label} linked — you can now sign in with it.`);
      window.history.replaceState(null, "", SETTINGS_HASH);
      onChanged();
    }
  }, [isJustLinked, toast, status.oidc.label, onChanged]);

  return (
    <PageShell page="settings" title="Settings">
      <div className="settings settings__body">
        <AgentWorkSection />
        <ReviewSection />
        <BuildHostsSection />
        <RepositoriesSection onChanged={onChanged} />
        <TokensSection />
        <AppearanceSection />
        <AlertsSection />
        <AccountSection status={status} onSignedOut={onSignedOut} />
      </div>
    </PageShell>
  );
}
