import type { CreatedToken } from "../../api/types";
import { Snippet } from "./Snippet";

/** Shows a new token's secret once, with ready-to-paste agent setup. */
export function TokenSecret({ token, onDone }: { token: CreatedToken; onDone: () => void }) {
  const origin = window.location.origin;
  const mcp = `claude mcp add --transport http ownership ${origin}/mcp --header "Authorization: Bearer ${token.secret}"`;
  const curl = [
    `curl -s -X POST ${origin}/api/v1/agent/claim \\`,
    `  -H "Authorization: Bearer ${token.secret}" \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -d '{"repo":"acme/api","wait_seconds":300}'`,
  ].join("\n");

  return (
    <div className="secret" role="status">
      <p className="secret__warning"><strong>Copy this token now.</strong> It won't be shown again.</p>
      <Snippet label="Token" value={token.secret} />
      <Snippet label="Add to Claude Code (MCP)" value={mcp} />
      <Snippet label="Or claim work with curl (repo is optional)" value={curl} />
      <button type="button" className="button button--secondary" onClick={onDone}>I've saved it</button>
    </div>
  );
}
