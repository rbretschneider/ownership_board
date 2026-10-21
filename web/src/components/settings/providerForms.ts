import type { BoardApi } from "../../api/boardApi";
import type { Connection, ConnectionProvider } from "../../api/types";
import type { AccountField } from "./ProviderTokenForm";

/** Everything that differs between the Forgejo and Bitbucket token forms. */
export interface TokenProviderSpec {
  provider: Exclude<ConnectionProvider, "github">;
  title: string;
  field: AccountField;
  tokenHint: string;
  submit: (api: BoardApi, account: string, token: string) => Promise<Connection>;
}

/** Forgejo / Gitea: base URL plus a read-only token. */
export const FORGEJO_FORM: TokenProviderSpec = {
  provider: "forgejo",
  title: "Forgejo / Gitea",
  field: {
    label: "Base URL", type: "url", isRequired: true, placeholder: "https://git.example.com", isUrlWarned: true,
    hint: "The address you open Forgejo or Gitea at.",
  },
  tokenHint: "Use a token with read:user and read:repository only. The board can't check that a token is read-only.",
  submit: (api, account, token) => api.connectForgejo({ base_url: account, token }),
};

/** Bitbucket Cloud: optional email plus a scoped token. */
export const BITBUCKET_FORM: TokenProviderSpec = {
  provider: "bitbucket",
  title: "Bitbucket",
  field: { label: "Email", type: "email", isRequired: false, placeholder: "you@example.com" },
  tokenHint:
    "A scoped API token with read:repository:bitbucket and read:user:bitbucket. Leave email empty for a repository or workspace access token.",
  submit: (api, account, token) => api.connectBitbucket({ email: account, token }),
};
