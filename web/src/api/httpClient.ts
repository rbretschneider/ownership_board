import { ApiError } from "./ApiError";

const API_BASE = "/api/v1";
const REFRESH_PATH = "/auth/refresh";
const HTTP_UNAUTHORIZED = 401;
const HTTP_NO_CONTENT = 204;
const NETWORK_ERROR_MESSAGE = "Could not reach the server. Check your connection and try again.";

/** HTTP verbs used by the client. */
export type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

/** A minimal JSON HTTP client bound to the API base path. */
export interface HttpClient {
  /**
   * Sends a request and parses the JSON response.
   * Resolves to `null` for `204 No Content`.
   */
  request<T>(method: HttpMethod, path: string, body?: unknown): Promise<T>;
}

/** Collaborators for {@link createHttpClient}. */
export interface HttpClientOptions {
  /** The fetch implementation (injected for tests). */
  fetchImpl: typeof fetch;
  /** Called when a refresh attempt fails and the session is gone. */
  onSessionExpired: () => void;
}

/**
 * Creates a client that retries a request once after refreshing the session
 * when the server answers 401.
 */
export function createHttpClient(options: HttpClientOptions): HttpClient {
  let refreshInFlight: Promise<boolean> | null = null;

  const send = async (method: HttpMethod, path: string, body?: unknown): Promise<Response> => {
    const init: RequestInit = {
      method,
      credentials: "same-origin",
      headers: body === undefined ? { Accept: "application/json" } : { Accept: "application/json", "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    };
    try {
      return await options.fetchImpl(API_BASE + path, init);
    } catch (cause) {
      throw new ApiError(0, "network", NETWORK_ERROR_MESSAGE, cause);
    }
  };

  const refresh = (): Promise<boolean> => {
    if (refreshInFlight === null) {
      refreshInFlight = send("POST", REFRESH_PATH)
        .then((res) => res.ok)
        .catch(() => false)
        .finally(() => {
          refreshInFlight = null;
        });
    }
    return refreshInFlight;
  };

  const request = async <T>(method: HttpMethod, path: string, body?: unknown): Promise<T> => {
    let res = await send(method, path, body);
    if (res.status === HTTP_UNAUTHORIZED && isRefreshable(path)) {
      const isRefreshed = await refresh();
      if (!isRefreshed) {
        options.onSessionExpired();
        throw await toApiError(res);
      }
      res = await send(method, path, body);
      if (res.status === HTTP_UNAUTHORIZED) {
        options.onSessionExpired();
      }
    }
    if (!res.ok) {
      throw await toApiError(res);
    }
    if (res.status === HTTP_NO_CONTENT) {
      return null as T;
    }
    return (await res.json()) as T;
  };

  return { request };
}

/** Auth endpoints answer 401 for bad credentials; refreshing would loop. */
function isRefreshable(path: string): boolean {
  return !path.startsWith("/auth/");
}

async function toApiError(res: Response): Promise<ApiError> {
  try {
    const payload = (await res.json()) as { error?: { code?: string; message?: string } };
    const code = payload.error?.code ?? "unknown";
    const message = payload.error?.message ?? `Request failed (${res.status})`;
    return new ApiError(res.status, code, message);
  } catch {
    return new ApiError(res.status, "unknown", `Request failed (${res.status})`);
  }
}
