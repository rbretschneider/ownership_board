/** Wire types for the ownership_board HTTP API (see docs/api.md). */

/** The four fixed lane ids. Finer progress lives in a ticket's stage. */
export type LaneId = "todo" | "in_progress" | "review" | "merged";

/** A ticket's place within its lane (each stage belongs to one lane). */
export type StageId = "refine" | "spec_review" | "ready" | "design" | "design_review" | "build" | "pr" | "done";

/** Who holds the ball for a ticket. */
export type Owner = "agent" | "human" | "nobody";

/** An approval checkpoint the owner resolves. */
export type Checkpoint = "spec" | "design";

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
  description: string;
}

/** A stage definition served by the API. */
export interface Stage {
  id: StageId;
  lane: LaneId;
  label: string;
  /** The board waits for the owner at this stage. */
  is_owner_review: boolean;
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
  /** Set while the provider no longer lists the repo. */
  missing_since: string | null;
  /** A person ticked "Treat as public" (e.g. a repo that may be open-sourced). */
  treat_as_public: boolean;
  /**
   * How agents must treat the repo. "private" only when a recent provider sync
   * said private and nobody ticked "Treat as public"; manual repos are public.
   */
  visibility: RepoVisibility;
  /** Agents wait for the owner to approve the spec before building. */
  require_spec_approval: boolean;
  /** Agents wait for the owner to approve the design before implementing. */
  require_design_approval: boolean;
}

/** Whether board content may appear in a repo's commits and PRs. */
export type RepoVisibility = "public" | "private";

/** Body of POST /repos (a manually added repository). */
export interface ManualRepoInput {
  full_name: string;
  url: string;
  description: string;
  color: string;
}

/** Body of PATCH /repos/{id}. */
export type RepoEdit = Partial<{
  color: string;
  description: string;
  treat_as_public: boolean;
  require_spec_approval: boolean;
  require_design_approval: boolean;
}>;

/** A provider the board can connect to for repo sync. */
export type ConnectionProvider = Exclude<Provider, "manual">;

/** Whether a provider is connected, and whether it needs attention. */
export type ConnectionStatus = "disconnected" | "connected" | "needs_reconnect";

/** Who configured a provider: an admin in Settings, or the server's env vars. */
export type ConnectionManager = "settings" | "server_env" | null;

/** Outcome of a provider's most recent sync. */
export interface SyncOutcome {
  at: string;
  /** "" when the sync succeeded. */
  error: string;
}

/** One row of GET /connections. */
export interface Connection {
  provider: ConnectionProvider;
  label: string;
  /** GitHub: a client ID is configured on the server. Always true for the others. */
  is_available: boolean;
  is_connected: boolean;
  status: ConnectionStatus;
  managed_by: ConnectionManager;
  /** Provider login, e.g. "octocat"; "" when unknown. */
  account: string;
  /** Forgejo instance URL; "" otherwise. */
  base_url: string;
  /** Bitbucket email; "" otherwise. */
  email: string;
  connected_at: string | null;
  repo_count: number;
  last_sync: SyncOutcome | null;
}

/** Answer to POST /connections/github/device. */
export interface DeviceFlow {
  flow_id: string;
  user_code: string;
  verification_uri: string;
  expires_at: string;
  interval_seconds: number;
}

/** Status of a GitHub device-flow poll. */
export type DevicePollStatus = "pending" | "confirm" | "expired" | "denied";

/** Answer to POST /connections/github/device/{flow_id}/poll. */
export interface DevicePoll {
  status: DevicePollStatus;
  interval_seconds: number;
  /** Set with `confirm`: the GitHub login that entered the code. */
  login?: string;
  /** Set with `confirm` when another login is connected now; "" otherwise. */
  replaces?: string;
  /** Optional reason with `denied`. */
  message?: string;
}

/** Body of PUT /connections/forgejo. */
export interface ForgejoConnectionInput {
  base_url: string;
  token: string;
}

/** Body of PUT /connections/bitbucket. */
export interface BitbucketConnectionInput {
  email: string;
  token: string;
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
  stage: StageId;
  /** "human" whenever needs_owner. */
  owner: Owner;
  /** Open questions, blocked, or a stage where the owner reviews. */
  needs_owner: boolean;
  /** Runs to a pull request with no checkpoints or questions; set by a delegate agent or the owner. */
  is_autonomous: boolean;
  /** "" when not blocked. Plain text, written by an agent. */
  blocked_reason: string;
  spec_revision: number;
  design_revision: number;
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
  | "released"
  | "work_link"
  | "decision"
  | "relation"
  | "stage"
  | "approved"
  | "changes_requested"
  | "blocked"
  | "unblocked"
  | "design";

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

/** Who recorded something, as the server saw them. */
export interface ActorRef {
  kind: ActorKind;
  name: string;
}

/** A PR or commit that implements a ticket. */
export interface WorkLink {
  id: number;
  kind: WorkLinkKind;
  /** Required for PRs; optional for commits ("" when absent). */
  url: string;
  /** Commits only ("" for PRs). */
  sha: string;
  /** Host of `url`, shown next to every link so look-alikes stand out. */
  host: string;
  added_by: ActorRef;
  added_at: string;
}

/** The two kinds of work link. */
export type WorkLinkKind = "pr" | "commit";

/** Body of POST /tickets/{id}/work-links. */
export interface WorkLinkInput {
  kind: WorkLinkKind;
  url?: string;
  sha?: string;
}

/** Where a decision came from. */
export type DecisionSource = "answer" | "agent" | "human" | "approval" | "chat";

/**
 * How much a decision binds: a person chose explicitly (binding), a person let
 * the AI decide (delegated), or an agent decided (agent).
 */
export type DecisionWeight = "binding" | "delegated" | "agent";

/** One entry of a ticket's decision log. Removed entries keep no content. */
export interface Decision {
  id: number;
  summary: string;
  rationale: string;
  alternatives: string[];
  source: DecisionSource;
  weight: DecisionWeight;
  decided_by: ActorRef;
  /** For answer-derived decisions: the agent that asked the question. */
  asked_by: string;
  question_id: number | null;
  supersedes_id: number | null;
  superseded_by_id: number | null;
  is_removed: boolean;
  at: string;
}

/** Body of POST /tickets/{id}/decisions. */
export interface DecisionInput {
  summary: string;
  rationale?: string;
  alternatives?: string[];
  supersedes_id?: number;
}

/** The stored kinds of ticket relation (always read from the "from" side). */
export type RelationKind = "caused_by" | "follow_up_of" | "duplicate_of" | "relates_to";

/** A relation as seen from one ticket. */
export interface Relation {
  kind: RelationKind;
  direction: "outgoing" | "incoming";
  /** Server label for this side, e.g. "caused by" or "caused". */
  label: string;
  ticket: { id: number; title: string; lane: LaneId; visibility: RepoVisibility };
  added_by: ActorRef;
  added_at: string;
}

/** Body of POST /tickets/{id}/relations. */
export interface RelationInput {
  kind: RelationKind;
  ticket_id: number;
}

/** The full ticket shape. */
export interface TicketDetail extends TicketSummary {
  body: string;
  spec: string;
  design: string;
  approved_spec_revision: number;
  approved_design_revision: number;
  /** Effective checkpoints for this ticket (no repo means both on). */
  gates: { spec: boolean; design: boolean };
  created_by: string;
  questions: Question[];
  timeline: TimelineEntry[];
  work_links: WorkLink[];
  decisions: Decision[];
  relations: Relation[];
}

/** Result of GET /board. */
export interface Board {
  lanes: Lane[];
  stages: Stage[];
  repos: Repo[];
  tickets: TicketSummary[];
}

/** Body of POST /tickets. Tickets always start in todo / refine. */
export interface CreateTicket {
  repo_id?: number | null;
  kind: Kind;
  title: string;
  body: string;
}

/** Body of POST /tickets/{id}/approve. */
export interface ApproveInput {
  checkpoint: Checkpoint;
  /** The revision the owner was shown; a mismatch answers 409. */
  revision: number;
}

/** Body of POST /tickets/{id}/request-changes. */
export interface RequestChangesInput extends ApproveInput {
  comment: string;
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
  is_autonomous: boolean;
}>;

/** One answer submitted by the human. */
export interface Answer {
  question_id: number;
  choice?: number | null;
  text?: string;
  deferred?: boolean;
  /** Filled by "Accept recommendations": the server records it as delegated, not a deliberate choice. */
  bulk?: boolean;
}

/** An agent access token (the secret is never listed). */
export interface Token {
  id: number;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at: string | null;
  /** May file autonomous tickets (specs agreed in a chat). */
  is_delegate: boolean;
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
