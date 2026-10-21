import { describe, expect, it, vi } from "vitest";
import { ApiError } from "./ApiError";
import { createHttpClient } from "./httpClient";

type Reply = { status: number; body?: unknown };

function fakeFetch(replies: Record<string, Reply[]>) {
  const calls: string[] = [];
  const impl = vi.fn(async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? "GET"} ${url}`;
    calls.push(key);
    const queue = replies[key];
    const reply = queue?.shift();
    if (reply === undefined) {
      throw new Error(`unexpected request ${key}`);
    }
    return new Response(reply.body === undefined ? null : JSON.stringify(reply.body), { status: reply.status });
  });
  return { impl: impl as unknown as typeof fetch, calls };
}

function clientWith(replies: Record<string, Reply[]>) {
  const fetcher = fakeFetch(replies);
  const onSessionExpired = vi.fn();
  const client = createHttpClient({ fetchImpl: fetcher.impl, onSessionExpired });
  return { client, calls: fetcher.calls, onSessionExpired };
}

const unauthorized: Reply = { status: 401, body: { error: { code: "unauthorized", message: "sign in" } } };

describe("httpClient", () => {
  it("returns parsed JSON on success", async () => {
    const { client } = clientWith({ "GET /api/v1/board": [{ status: 200, body: { tickets: [] } }] });
    await expect(client.request("GET", "/board")).resolves.toEqual({ tickets: [] });
  });

  it("refreshes once and retries after a 401", async () => {
    const { client, calls } = clientWith({
      "GET /api/v1/board": [unauthorized, { status: 200, body: { ok: true } }],
      "POST /api/v1/auth/refresh": [{ status: 200, body: {} }],
    });
    await expect(client.request("GET", "/board")).resolves.toEqual({ ok: true });
    expect(calls).toEqual(["GET /api/v1/board", "POST /api/v1/auth/refresh", "GET /api/v1/board"]);
  });

  it("reports an expired session when the refresh fails", async () => {
    const { client, onSessionExpired } = clientWith({
      "GET /api/v1/board": [unauthorized],
      "POST /api/v1/auth/refresh": [unauthorized],
    });
    await expect(client.request("GET", "/board")).rejects.toBeInstanceOf(ApiError);
    expect(onSessionExpired).toHaveBeenCalledOnce();
  });

  it("does not refresh for auth endpoints", async () => {
    const { client, calls } = clientWith({ "POST /api/v1/auth/login": [unauthorized] });
    await expect(client.request("POST", "/auth/login", {})).rejects.toMatchObject({ code: "unauthorized", message: "sign in" });
    expect(calls).toEqual(["POST /api/v1/auth/login"]);
  });

  it("shares one refresh between concurrent 401s", async () => {
    const { client, calls } = clientWith({
      "GET /api/v1/board": [unauthorized, { status: 200, body: 1 }],
      "GET /api/v1/projects": [unauthorized, { status: 200, body: 2 }],
      "POST /api/v1/auth/refresh": [{ status: 200, body: {} }],
    });
    await Promise.all([client.request("GET", "/board"), client.request("GET", "/projects")]);
    expect(calls.filter((c) => c.includes("refresh"))).toHaveLength(1);
  });

  it("surfaces the envelope message on errors", async () => {
    const { client } = clientWith({
      "GET /api/v1/tickets/9": [{ status: 404, body: { error: { code: "not_found", message: "ticket 9 does not exist" } } }],
    });
    await expect(client.request("GET", "/tickets/9")).rejects.toMatchObject({ status: 404, message: "ticket 9 does not exist" });
  });

  it("resolves null for 204", async () => {
    const { client } = clientWith({ "DELETE /api/v1/tickets/3": [{ status: 204 }] });
    await expect(client.request("DELETE", "/tickets/3")).resolves.toBeNull();
  });
});
