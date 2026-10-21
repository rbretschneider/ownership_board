/** Wire types for the ownership_board HTTP API (see docs/api.md). */

/** The six fixed lane ids. */
export type LaneId = "new" | "answers" | "ready" | "in_progress" | "review" | "done";

/** Who holds the ball in a lane. */
export type Owner = "agent" | "human" | "nobody";

/** Ticket kind. */
export type Kind = "story" | "bug";

/** A signed-in human account. */
export interface User {
  id: number;
  name: string;
  email: string;
  is_admin: boolean;
  /** Whether this account has linked an SSO identity. */
  sso_linked: boolean;
}

/** The caller identity returned by GET /me. */
export interface Actor {
  kind: "human" | "agent";
  id: number;
  name: string;
}

/** Result of GET /auth/status. */
export interface AuthStatus {
  setup_required: boolean;
  password_login: boolean;
  oidc: { enabled: boolean; label: string };
  user: User | null;
}

/** A lane definition served by the API. */
export interface Lane {
  id: LaneId;
  label: string;
  owner: Owner;
  description: string;
}

/** Where a repository comes from. */
export type Provider = "github" | "bitbucket" | "forgejo" | "manual";

/** A code repository tickets can target. */
export interface Repo {
  id: number;
  provider: Provider;
  /** e.g. "acme/api". */
  full_name: string;
  /** Last path segment of `full_name`. */
  name: string;
  url: string;
  description: string;
  /** "" or `#rrggbb`. */
  color: string;
  default_branch: string;
  is_private: boolean;
  is_archived: boolean;
  synced_at: string | null;
}

/** Body of POST /repos (a manually added repository). */
export interface ManualRepoInput {
  full_name: string;
  url: string;
  description: string;
  color: string;
}

/** Body of PATCH /repos/{id}. */
export type RepoEdit = Partial<{ color: string; description: string }>;

/** Status of a configured repository provider. */
export interface RepoSource {
  provider: Provider;
  label: string;
  is_configured: boolean;
  repo_count: number;
  last_synced_at: string | null;
  last_error: string;
}

/** An agent's claim on a ticket. */
export interface Lease {
  holder: string;
  expires_at: string;
}

/** The compact ticket shape used on the board. */
export interface TicketSummary {
  id: number;
  /** null = no repo yet; the AI infers it or asks. */
  repo_id: number | null;
  kind: Kind;
  title: string;
  lane: LaneId;
  owner: Owner;
  pr_url: string;
  branch: string;
  open_questions: number;
  lease: Lease | null;
  created_at: string;
  updated_at: string;
  lane_changed_at: string;
}

/** An answer recorded against a question. */
export interface RecordedAnswer {
  choice: number | null;
  text: string;
  deferred: boolean;
  by: string;
  at: string;
}

/** A structured question asked by the agent. */
export interface Question {
  id: number;
  round: number;
  prompt: string;
  options: string[];
  recommended: number | null;
  asked_by: string;
  asked_at: string;
  answer: RecordedAnswer | null;
}

/** Timeline event types. */
export type TimelineType =
  | "created"
  | "moved"
  | "comment"
  | "spec"
  | "questions"
  | "answers"
  | "edited"
  | "claimed"
  | "released";

/** Who performed a timeline event. */
export type ActorKind = "human" | "agent" | "system";

/** One entry in a ticket's history. */
export interface TimelineEntry {
  id: number;
  type: TimelineType;
  at: string;
  actor: { kind: ActorKind; name: string };
  body: string;
  data: Record<string, unknown>;
}

/** The full ticket shape. */
export interface TicketDetail extends TicketSummary {
  body: string;
  spec: string;
  created_by: string;
  questions: Question[];
  timeline: TimelineEntry[];
}

/** Result of GET /board. */
export interface Board {
  lanes: Lane[];
  repos: Repo[];
  tickets: TicketSummary[];
}

/** Body of POST /tickets. */
export interface CreateTicket {
  repo_id?: number | null;
  kind: Kind;
  title: string;
  body: string;
  lane?: LaneId;
}

/** Body of PATCH /tickets/{id}. */
export type EditTicket = Partial<{
  /** null clears the repo. */
  repo_id: number | null;
  kind: Kind;
  title: string;
  body: string;
  pr_url: string;
  branch: string;
}>;

/** One answer submitted by the human. */
export interface Answer {
  question_id: number;
  choice?: number | null;
  text?: string;
  deferred?: boolean;
}

/** An agent access token (the secret is never listed). */
export interface Token {
  id: number;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at: string | null;
}

/** A freshly created token, including its one-time secret. */
export interface CreatedToken extends Token {
  secret: string;
}

/** Payload of a server-sent board event. */
export interface BoardEvent {
  cursor: number;
  type: TimelineType | "deleted";
  ticket_id: number;
}
