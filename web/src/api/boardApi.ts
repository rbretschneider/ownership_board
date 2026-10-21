import type { HttpClient } from "./httpClient";
import type {
  Answer,
  AuthStatus,
  Board,
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
  ForgejoConnectionInput,
  TicketDetail,
  Token,
  User,
} from "./types";

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
  deleteTicket(id: number): Promise<void>;
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
  createToken(name: string): Promise<CreatedToken>;
  revokeToken(id: number): Promise<void>;
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
    deleteTicket: (id) => http.request("DELETE", t(id)),
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
    createToken: (name) => http.request("POST", "/tokens", { name }),
    revokeToken: (id) => http.request("DELETE", `/tokens/${id}`),
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

/** Path of one GitHub device flow; the id is opaque, so it is encoded. */
export function deviceFlowPath(flowId: string): string {
  return `/connections/github/device/${encodeURIComponent(flowId)}`;
}
