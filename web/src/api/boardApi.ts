import type { HttpClient } from "./httpClient";
import type {
  Answer,
  ApproveInput,
  AuthStatus,
  Board,
  BoardSettings,
  CreatedToken,
  CreateTicket,
  EditTicket,
  LaneId,
  ManualRepoInput,
  Repo,
  RepoEdit,
  BitbucketConnectionInput,
  Connection,
  ConnectionProvider,
  DeviceFlow,
  DevicePoll,
  DecisionInput,
  ForgejoConnectionInput,
  RelationInput,
  RelationKind,
  RequestChangesInput,
  TicketDetail,
  TicketSummary,
  Token,
  User,
  WorkLinkInput,
} from "./types";

/** Query for GET /work-links: a commit sha prefix (7–40 hex) or a PR URL. */
export type WorkLookup = { sha: string } | { url: string };

/**
 * Board filter by repository: all tickets, tickets without a repo, or one
 * repo's tickets.
 */
export type RepoFilter = { kind: "all" } | { kind: "none" } | { kind: "repo"; id: number };

/** Typed access to every endpoint the web UI uses. */
export interface BoardApi {
  authStatus(): Promise<AuthStatus>;
  setup(input: { name: string; email: string; password: string }): Promise<User>;
  login(input: { email: string; password: string }): Promise<User>;
  logout(): Promise<void>;
  board(filter: RepoFilter): Promise<Board>;
  ticket(id: number): Promise<TicketDetail>;
  createTicket(input: CreateTicket): Promise<TicketDetail>;
  editTicket(id: number, input: EditTicket): Promise<TicketDetail>;
  moveTicket(id: number, lane: LaneId, afterId: number | null): Promise<TicketDetail>;
  comment(id: number, body: string): Promise<TicketDetail>;
  answer(id: number, answers: Answer[]): Promise<TicketDetail>;
  /** Resolves a checkpoint; 409 when the stage or revision changed. */
  approve(id: number, input: ApproveInput): Promise<TicketDetail>;
  /** Sends a checkpoint back with a required comment; 409 as for approve. */
  requestChanges(id: number, input: RequestChangesInput): Promise<TicketDetail>;
  /** Clears Blocked; an empty comment sends none. */
  unblock(id: number, comment: string): Promise<TicketDetail>;
  deleteTicket(id: number): Promise<void>;
  addWorkLink(id: number, input: WorkLinkInput): Promise<TicketDetail>;
  removeWorkLink(id: number, linkId: number): Promise<TicketDetail>;
  /** Tickets a commit or PR belongs to, from recorded work links only. */
  findWork(lookup: WorkLookup): Promise<TicketSummary[]>;
  addDecision(id: number, input: DecisionInput): Promise<TicketDetail>;
  removeDecision(id: number, decisionId: number): Promise<TicketDetail>;
  addRelation(id: number, input: RelationInput): Promise<TicketDetail>;
  removeRelation(id: number, kind: RelationKind, otherId: number): Promise<TicketDetail>;
  repos(): Promise<Repo[]>;
  addRepo(input: ManualRepoInput): Promise<Repo>;
  editRepo(id: number, input: RepoEdit): Promise<Repo>;
  connections(): Promise<Connection[]>;
  syncConnections(): Promise<Connection[]>;
  startGitHubDevice(): Promise<DeviceFlow>;
  pollGitHubDevice(flowId: string): Promise<DevicePoll>;
  confirmGitHubDevice(flowId: string): Promise<Connection>;
  cancelGitHubDevice(flowId: string): Promise<void>;
  connectForgejo(input: ForgejoConnectionInput): Promise<Connection>;
  connectBitbucket(input: BitbucketConnectionInput): Promise<Connection>;
  disconnect(provider: ConnectionProvider): Promise<void>;
  tokens(): Promise<Token[]>;
  createToken(name: string, isDelegate: boolean): Promise<CreatedToken>;
  revokeToken(id: number): Promise<void>;
  settings(): Promise<BoardSettings>;
  editSettings(input: Partial<BoardSettings>): Promise<BoardSettings>;
}

/** Builds the {@link BoardApi} on top of an {@link HttpClient}. */
export function createBoardApi(http: HttpClient): BoardApi {
  const t = (id: number): string => `/tickets/${id}`;
  return {
    authStatus: () => http.request("GET", "/auth/status"),
    setup: (input) => http.request("POST", "/auth/setup", input),
    login: (input) => http.request("POST", "/auth/login", input),
    logout: () => http.request("POST", "/auth/logout"),
    board: (filter) => http.request("GET", `/board${boardQuery(filter)}`),
    ticket: (id) => http.request("GET", t(id)),
    createTicket: (input) => http.request("POST", "/tickets", input),
    editTicket: (id, input) => http.request("PATCH", t(id), input),
    moveTicket: (id, lane, afterId) => http.request("POST", `${t(id)}/move`, { lane, after_id: afterId }),
    comment: (id, body) => http.request("POST", `${t(id)}/comments`, { body }),
    answer: (id, answers) => http.request("POST", `${t(id)}/answers`, { answers }),
    approve: (id, input) => http.request("POST", `${t(id)}/approve`, input),
    requestChanges: (id, input) => http.request("POST", `${t(id)}/request-changes`, input),
    unblock: (id, comment) => http.request("POST", `${t(id)}/unblock`, comment === "" ? {} : { comment }),
    deleteTicket: (id) => http.request("DELETE", t(id)),
    addWorkLink: (id, input) => http.request("POST", `${t(id)}/work-links`, input),
    removeWorkLink: (id, linkId) => http.request("DELETE", `${t(id)}/work-links/${linkId}`),
    findWork: (lookup) => http.request("GET", `/work-links${workLookupQuery(lookup)}`),
    addDecision: (id, input) => http.request("POST", `${t(id)}/decisions`, input),
    removeDecision: (id, decisionId) => http.request("DELETE", `${t(id)}/decisions/${decisionId}`),
    addRelation: (id, input) => http.request("POST", `${t(id)}/relations`, input),
    removeRelation: (id, kind, otherId) => http.request("DELETE", `${t(id)}/relations/${kind}/${otherId}`),
    repos: () => http.request("GET", "/repos"),
    addRepo: (input) => http.request("POST", "/repos", input),
    editRepo: (id, input) => http.request("PATCH", `/repos/${id}`, input),
    connections: () => http.request("GET", "/connections"),
    syncConnections: () => http.request("POST", "/connections/sync"),
    startGitHubDevice: () => http.request("POST", "/connections/github/device"),
    pollGitHubDevice: (flowId) => http.request("POST", `${deviceFlowPath(flowId)}/poll`),
    confirmGitHubDevice: (flowId) => http.request("POST", `${deviceFlowPath(flowId)}/confirm`),
    cancelGitHubDevice: (flowId) => http.request("DELETE", deviceFlowPath(flowId)),
    connectForgejo: (input) => http.request("PUT", "/connections/forgejo", input),
    connectBitbucket: (input) => http.request("PUT", "/connections/bitbucket", input),
    disconnect: (provider) => http.request("DELETE", `/connections/${provider}`),
    tokens: () => http.request("GET", "/tokens"),
    createToken: (name, isDelegate) => http.request("POST", "/tokens", { name, is_delegate: isDelegate }),
    revokeToken: (id) => http.request("DELETE", `/tokens/${id}`),
    settings: () => http.request("GET", "/settings"),
    editSettings: (input) => http.request("PATCH", "/settings", input),
  };
}

/** Query string for GET /board. */
export function boardQuery(filter: RepoFilter): string {
  switch (filter.kind) {
    case "all":
      return "";
    case "none":
      return "?repo=none";
    case "repo":
      return `?repo=${filter.id}`;
  }
}

/** Query string for GET /work-links. */
export function workLookupQuery(lookup: WorkLookup): string {
  return "sha" in lookup ? `?sha=${encodeURIComponent(lookup.sha)}` : `?url=${encodeURIComponent(lookup.url)}`;
}

/** Path of one GitHub device flow; the id is opaque, so it is encoded. */
export function deviceFlowPath(flowId: string): string {
  return `/connections/github/device/${encodeURIComponent(flowId)}`;
}
