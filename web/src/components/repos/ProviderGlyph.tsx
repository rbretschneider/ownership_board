import type { Provider } from "../../api/types";
import { PROVIDER_LABELS } from "../../lib/repos";

const GLYPHS: Record<Provider, string> = { github: "GH", bitbucket: "BB", forgejo: "FJ", manual: "M" };

/** A tiny provider badge; the full provider name is available to screen readers. */
export function ProviderGlyph({ provider }: { provider: Provider }) {
  return (
    <span className={`provider provider--${provider}`} title={PROVIDER_LABELS[provider]}>
      <span aria-hidden="true">{GLYPHS[provider]}</span>
      <span className="visually-hidden">{PROVIDER_LABELS[provider]}</span>
    </span>
  );
}
