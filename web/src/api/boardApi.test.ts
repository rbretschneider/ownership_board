import { expect, it, vi } from "vitest";
import { createBoardApi } from "./boardApi";
it("uses session-aware host endpoints and preserves omitted versus empty edits", async () => {
  const request = vi.fn().mockResolvedValue({});
  const api = createBoardApi({ request });
  await api.buildHosts();
  await api.issueBuildHostCode();
  await api.approveBuildHost(2, { capabilities: [] });
  await api.editBuildHost(2, { enabled: false });
  await api.revokeBuildHost(2);
  await api.buildHostEvents(2);
  await api.registeredRunners();
  expect(request.mock.calls).toEqual([
    ["GET", "/build-hosts"],
    ["POST", "/build-hosts/pairing-codes", {}],
    ["POST", "/build-hosts/2/approve", { capabilities: [] }],
    ["PATCH", "/build-hosts/2", { enabled: false }],
    ["POST", "/build-hosts/2/revoke", {}],
    ["GET", "/build-hosts/2/events?latest=true&limit=20"],
    ["GET", "/build-hosts/runners"],
  ]);
});

it("requests deployment and forwards the authoritative detail", async () => {
  const detail = {id: 7, stage: "deploying"};
  const request = vi.fn().mockResolvedValue(detail);
  expect(await createBoardApi({request}).deployTicket(7)).toBe(detail);
  expect(request).toHaveBeenCalledExactlyOnceWith("POST", "/tickets/7/deploy", {});
});
