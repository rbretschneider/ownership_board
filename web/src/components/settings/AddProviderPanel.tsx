import { ChevronDown } from "lucide-react";
import { useId, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Connection } from "../../api/types";
import { hasCard } from "../../lib/connections";
import { BITBUCKET_FORM, FORGEJO_FORM, type TokenProviderSpec } from "./providerForms";
import { ProviderTokenForm } from "./ProviderTokenForm";

/** Props for {@link AddProviderPanel}. */
export interface AddProviderPanelProps {
  connections: Connection[];
  onConnected: (connection: Connection) => void;
}

/**
 * The collapsed "Add another provider" disclosure: token forms for Forgejo and
 * Bitbucket, plus a note when the server has no GitHub client ID.
 */
export function AddProviderPanel({ connections, onConnected }: AddProviderPanelProps) {
  const api = useApi();
  const panelId = useId();
  const [isOpen, setOpen] = useState(false);
  const [chosen, setChosen] = useState<TokenProviderSpec | null>(null);
  const unset = (provider: Connection["provider"]) => !connections.some((c) => c.provider === provider && hasCard(c));
  const github = connections.find((c) => c.provider === "github");
  const offers = [
    { spec: FORGEJO_FORM, isShown: unset("forgejo") },
    { spec: BITBUCKET_FORM, isShown: unset("bitbucket") },
  ].filter((offer) => offer.isShown);
  const isGitHubMissing = github !== undefined && !github.is_available;

  if (offers.length === 0 && !isGitHubMissing) {
    return null;
  }
  return (
    <div className="disclosure">
      <button type="button" className="link-button disclosure__toggle" aria-expanded={isOpen} aria-controls={panelId} onClick={() => setOpen(!isOpen)}>
        <ChevronDown size={16} aria-hidden="true" className={isOpen ? "disclosure__chevron--open" : ""} />
        Add another provider
      </button>
      {isOpen && (
        <div id={panelId} className="disclosure__panel">
          {chosen === null && (
            <div className="provider-picker">
              {offers.map((offer) => (
                <button key={offer.spec.provider} type="button" className="button button--secondary" onClick={() => setChosen(offer.spec)}>
                  {offer.spec.title}
                </button>
              ))}
            </div>
          )}
          {chosen !== null && (
            <ProviderTokenForm
              idPrefix={`connect-${chosen.provider}`}
              title={`Connect ${chosen.title}`}
              field={chosen.field}
              tokenHint={chosen.tokenHint}
              submitLabel="Connect"
              onSubmit={(account, token) => chosen.submit(api, account, token)}
              onConnected={(connection) => {
                setChosen(null);
                setOpen(false);
                onConnected(connection);
              }}
              onCancel={() => setChosen(null)}
            />
          )}
          {isGitHubMissing && (
            <p className="field__hint">GitHub sign-in needs GITHUB_APP_CLIENT_ID on the server (see docs/agents.md).</p>
          )}
        </div>
      )}
    </div>
  );
}
