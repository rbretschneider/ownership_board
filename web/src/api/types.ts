/** Wire types for the ownership_board HTTP API (see docs/api.md). */

/** The four fixed lane ids. Finer progress lives in a ticket's stage. */
export type LaneId = "todo" | "in_progress" | "merged" | "deployed";

/** A ticket's place within its lane (each stage belongs to one lane). */
export type StageId = "refine" | "spec_review" | "ready" | "design" | "design_review" | "build" | "pr" | "done" | "deploying" | "deployed";

export type DeployMode = "off" | "agent";

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
  overlap_ignore: string[];
  build_targets: RepoBuildTarget[];
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
  /** The owner lets agents merge this repo's green, review-clean PRs themselves. */
  allow_agent_merge: boolean;
  deploy_mode: DeployMode;
  /** Merges between architecture health checks; 0 turns the check off. */
  health_check_every: number;
  /** The last merged ticket a health check covered; null before the first check. */
  health_checked_through_ticket: number | null;
  /** When that check ran; null before the first check. */
  health_checked_at: string | null;
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

export interface RepoBuildTarget {
  os: BuildHostOS;
  capabilities: string[];
  preset: "dotnet" | "cmake" | "script";
}

/** Body of PATCH /repos/{id}. */
export type RepoEdit = Partial<{
  overlap_ignore: string[];
  build_targets: RepoBuildTarget[];
  color: string;
  description: string;
  treat_as_public: boolean;
  require_spec_approval: boolean;
  require_design_approval: boolean;
  allow_agent_merge: boolean;
  deploy_mode: DeployMode;
  health_check_every: number;
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
  /** Server-owned marker for a scheduled architecture review. */
  readonly is_health_check?: boolean;
  /** "" when not blocked. Plain text, written by an agent. */
  blocked_reason: string;
  spec_revision: number;
  design_revision: number;
  pr_url: string;
  branch: string;
  open_questions: number;
  /** Tickets this one depends on that are not merged yet; agents cannot claim it until this is empty. */
  waiting_on: number[];
  work_model?: string;
  lease: Lease | null;
  created_at: string;
  updated_at: string;
  lane_changed_at: string;
  /** Passes through each delivery step, derived from the ticket's history. Absent from older servers. */
  attempts?: StageAttempts;
}

/** Read-only step counts the board derives from stage moves and work-model reports. 0 = never started. */
export interface StageAttempts {
  /** Entries into build plus returns after a failed Windows run or review. */
  build: number;
  /** Windows verification runs, one per commit sent. */
  test: number;
  test_failures: number;
  /** Fresh-context adversarial review rounds. */
  review: number;
  review_fixes: number;
  /** Entries into the PR stage. */
  pr: number;
  /** Returns from an open PR to build. */
  pr_repairs: number;
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
  | "design"
  | "dependencies_met";

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
export type RelationKind = "caused_by" | "follow_up_of" | "duplicate_of" | "relates_to" | "depends_on";

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
  /** Tickets in the same repo the new one waits for. */
  depends_on?: number[];
  /** Start on autopilot: no checkpoints or questions, straight to a pull request. Needs a repo. */
  is_autonomous?: boolean;
}

/** Board-wide settings (GET and PATCH /settings; only people change them). */
export interface BoardSettings {
  windows_runner_enabled: boolean;
  windows_parallel_builds: number;
  windows_parallel_build_limit: number;
  codex_workers: number;
  codex_worker_limit: number;
  claude_workers: number;
  claude_worker_limit: number;
  codex_weekly_limit: number;
  claude_weekly_limit: number;
  /** How many autonomous tickets may be in design or build at once (1–10). */
  autonomous_in_flight_cap: number;
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

/** Cached provider status only; collecting this never starts an inference turn. */
export interface AccountUsage {
 accounts: { provider: "codex" | "claude"; observed_at: string | null; windows: {label: string; used_percent: number | null; resets_at: string | null}[] }[];
}

export type RunnerPhase = "queued" | "preparing" | "building" | "testing" | "verifying";
export type RunnerPreset = "dotnet" | "cmake" | "script";

/** One Windows build slot and the job it is running. */
export interface RunnerSlot {
 slot: number;
 ticket_id: number | null;
 repo: string;
 sha: string;
 preset: RunnerPreset;
 phase: RunnerPhase;
 /** The current step, e.g. "ASan · testing" or "waiting for desktop"; may be empty. */
 detail: string;
 since: string | null;
 updated_at: string | null;
 /** A newer commit of the same ticket replaced this run; the host stops it. */
 orphan: boolean;
}

/** A job waiting for a free slot, in claim order. */
export interface RunnerQueued {
 position: number;
 ticket_id: number | null;
 repo: string;
 sha: string;
 preset: RunnerPreset;
 queued_at: string | null;
 deferred_until: string | null;
}

/** The Windows build host's live state; observed_at is null when unknown. */
export interface RunnersSnapshot {
 hosts?: (Omit<RunnersSnapshot, "hosts"> & {id: number; os: BuildHostOS; online: boolean})[];
 observed_at: string | null;
 host: string;
 enabled: boolean;
 parallel_builds: number;
 /** Why the slots are not taking board work right now; empty when they are. */
 blocked: "" | "github-actions" | "disabled" | "low-disk" | "no-settings" | "drained";
 blocked_since: string | null;
 slots: RunnerSlot[];
 queue: RunnerQueued[];
}

/** One CI check on a board pull request, as the merge sweep last saw it. */
export interface PRCheck {
 name: string;
 status: string;
 /** SUCCESS, FAILURE, CANCELLED, ...; empty while it runs. */
 conclusion: string;
 started_at: string | null;
 completed_at: string | null;
}

/** What the merge sweep decided about a PR on its last run. */
export type PRVerdict = "running" | "no_checks" | "draft" | "checking_mergeable" | "checks_incomplete" | "green_owner" | "merged" | "merge_failed" | "closed" | "repair";

/** One open board pull request and what it is waiting for. */
export interface PRStatus {
 ticket_id: number;
 url: string;
 verdict: PRVerdict;
 /** Why a repair was queued; empty unless verdict is "repair". */
 reason: "" | "conflict" | "changes_requested" | "checks_failed" | "unresolved";
 /** The repo lets agents merge green PRs. */
 may_merge: boolean;
 /** The sweep acts on this PR (autonomous ticket or merge grant). */
 acts: boolean;
 gl_waived: boolean;
 draft: boolean;
 mergeable: string;
 review: string;
 head: string;
 /** Published past the fix-round limit; never merged automatically. */
 unresolved: boolean;
 /** Published for the owner's hands-on check (evidence no agent can produce); never merged automatically. */
 manual_check: boolean;
 checks: PRCheck[];
}

/** The merge sweep's view of every open board PR; observed_at is null when unknown. */
export interface PRStatusSnapshot {
 observed_at: string | null;
 prs: PRStatus[];
}

/** Durations in hours; with `count` 0 the other fields mean "no data". */
export interface Distribution {
  count: number;
  median_hours: number;
  p90_hours: number;
}

/** A tally for one name, such as a model or an action. */
export interface NamedCount {
  name: string;
  count: number;
}

/** Delivery metrics for one group of tickets (GET /stats). */
export interface StatsSegment {
  /** Reached done in the last 24 hours, whatever the window. */
  merged_24h: number;
  merged: number;
  prs_opened: number;
  created: number;
  daily: { date: string; merged: number }[];
  lead_time: Distribution;
  cycle_time: Distribution;
  stage_times: (Distribution & { stage: StageId })[];
  /** Open tickets per stage now. */
  wip: { stage: StageId; lane: LaneId; count: number }[];
  blocked: number;
  needs_owner: number;
  repair_rounds: number;
  repaired_tickets: number;
  changes_requested: number;
  models: NamedCount[];
  claims: NamedCount[];
  agents: NamedCount[];
}

/** One repo's delivery for one group of tickets. */
export interface RepoDelivery {
  merged: number;
  prs_opened: number;
  open: number;
}

/** Result of GET /stats: autonomous tickets versus the owner's own. */
export interface StatsReport {
  generated_at: string;
  window_days: number;
  since: string;
  all: StatsSegment;
  autonomous: StatsSegment;
  owner: StatsSegment;
  repos: StatsRepo[];
}

/** Current architecture review status, independent of the delivery period. */
export interface RepoHealth {
  every: number;
  merges_since_check: number;
  next_in: number | null;
  last_check_ticket: number | null;
  /** Filing/checkpoint time, rather than completion time. */
  last_checked_at: string | null;
  open_follow_ups: number;
}

/** Delivery counts and current review status for one repository. */
export interface StatsRepo {
  repo_id: number;
  name: string;
  autonomous: RepoDelivery;
  owner: RepoDelivery;
  health: RepoHealth | null;
}

export type BuildHostOS = "windows" | "linux" | "macos";
export interface BuildHostFacts {
 os?: BuildHostOS; os_version?: string; arch?: string; hostname?: string;
 cpu_model?: string; cores?: number; threads?: number; ram_bytes?: number;
 disks?: {mount:string;total:number;free:number;is_build_volume:boolean}[];
 gpus?: {name:string;driver:string;opengl_version:string}[];
 toolchains?: Record<string,string>; interactive_session?: boolean; build_user_is_admin?: boolean;
}
export interface BuildHost {
 id:number; name:string; os:BuildHostOS; arch:string; status:"pending"|"approved"|"revoked";
 capabilities:string[]; max_parallel:number; enabled:boolean; online:boolean; last_seen_at:string|null;
 facts:BuildHostFacts; agent_version:string; free_disk_bytes?:number|null; revoked_at?:string|null;
}
export interface BuildHostEdit { enabled?:boolean; max_parallel?:number; capabilities?:string[] }
export interface BuildHostPairingCode { code:string; expires_at:string; public_base_url:string }
export interface BuildHostEvent { id:number; event_type:string; created_at:string; data:{before?:Record<string,string>;after?:Record<string,string>} }
export interface RegisteredRunnerJob {
 id:string;ticket_id:number;repo:string;sha:string;preset:string;status:string;phase:string;detail:string;started_at:string|null;attempt:number;
}
export interface RegisteredRunnersSnapshot {
 observed_at:string;
 hosts:{id:number;name:string;os:BuildHostOS;enabled:boolean;max_parallel:number;online:boolean;last_seen_at:string|null;jobs:RegisteredRunnerJob[]}[];
 queued_by_os:Record<BuildHostOS,number>;
}
