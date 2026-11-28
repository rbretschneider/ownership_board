import type { BoardApi } from "../../api/boardApi";
import type { Connection, ConnectionProvider } from "../../api/types";
import type { AccountField, FormOption } from "./ProviderTokenForm";

/** Everything that differs between the Forgejo and Bitbucket token forms. */
export interface TokenProviderSpec {
  provider: Exclude<ConnectionProvider, "github">;
  title: string;
  field: AccountField;
  tokenHint: string;
  /** An extra opt-in checkbox, when the provider has one. */
  option?: FormOption;
  submit: (api: BoardApi, account: string, token: string, isOptionOn: boolean) => Promise<Connection>;
}

/** Forgejo / Gitea: base URL plus a token, optionally lent to board agents. */
export const FORGEJO_FORM: TokenProviderSpec = {
  provider: "forgejo",
  title: "Forgejo / Gitea",
  field: {
    label: "Base URL", type: "url", isRequired: true, placeholder: "https://git.example.com", isUrlWarned: true,
    hint: "The address you open Forgejo or Gitea at.",
  },
  tokenHint:
    "For repo sync only: read:user and read:repository. If agents may use it: read:user and write:repository " +
    "(pushes, pull requests and merges), ideally on a bot account. The board can't check a token's scopes.",
  option: {
    label: "Let board agents use this token",
    hint: "Workers clone, push branches and open pull requests with it, and the merge sweep merges green PRs where a repo allows it. " +
      "Branch protection on Forgejo stays the real guard.",
  },
  submit: (api, account, token, isOptionOn) => api.connectForgejo({ base_url: account, token, allow_agent_access: isOptionOn }),
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
