import { ExternalLink } from "lucide-react";
import { ProviderGlyph } from "../repos/ProviderGlyph";

/**
 * Prefilled GitHub App registration: a read-only (Metadata) app with webhooks
 * off. GitHub has no URL parameter for Device Flow, so the steps say to tick it.
 */
export const GITHUB_APP_REGISTRATION_URL =
  "https://github.com/settings/apps/new?name=ownership-board&description=Read-only%20repo%20list%20for%20a%20private%20task%20board&url=https%3A%2F%2Fgithub.com%2Frbretschneider%2Fownership_board&public=false&webhook_active=false&metadata=read";

/** Shown while the server has no GitHub client ID: how to set GitHub up once. */
export function GitHubSetupCard() {
  return (
    <div className="connection github-setup">
      <div className="connection__head">
        <ProviderGlyph provider="github" />
        <div className="connection__text">
          <span className="connection__label">Set up GitHub</span>
          <span className="connection__account">
            GitHub needs a one-time app registration so the board can sign in like GitHub Desktop.
          </span>
        </div>
      </div>
      <a className="button button--primary button--block" href={GITHUB_APP_REGISTRATION_URL} target="_blank" rel="noopener noreferrer">
        Register the app on GitHub
        <ExternalLink size={14} aria-hidden="true" />
      </a>
      <ol className="github-setup__steps">
        <li>GitHub may ask you to change the name to something unique.</li>
        <li>Tick <strong>Enable Device Flow</strong>.</li>
        <li>Create it, then <strong>Generate a private key</strong> and delete the downloaded file; the board never uses it.</li>
        <li><strong>Install App</strong> → your account → <strong>All repositories</strong>.</li>
        <li>Put the app's Client ID in <code>GITHUB_APP_CLIENT_ID</code> on the server and restart the board.</li>
      </ol>
    </div>
  );
}
