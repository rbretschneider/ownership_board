import { describe, expect, it } from "vitest";
import { parseHash } from "./hashRoute";

describe("parseHash", () => {
  it("defaults to the board", () => {
    expect(parseHash("").name).toBe("board");
    expect(parseHash("#/").name).toBe("board");
    expect(parseHash("#/nonsense").name).toBe("board");
  });
  it("parses a ticket id", () => {
    expect(parseHash("#/t/42")).toMatchObject({ name: "ticket", ticketId: 42 });
  });
  it("parses settings with params", () => {
    const route = parseHash("#/settings?linked=1");
    expect(route.name).toBe("settings");
    expect(route.params.get("linked")).toBe("1");
  });
  it("parses a login error", () => {
    expect(parseHash("#/login?error=sso_not_linked").params.get("error")).toBe("sso_not_linked");
  });
});
