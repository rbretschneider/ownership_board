import { describe, expect, it } from "vitest";
import { edgeSpeed } from "./autoScroll";

describe("edgeSpeed", () => {
  it("is zero away from the edges", () => {
    expect(edgeSpeed(500, 0, 1000)).toBe(0);
  });
  it("scrolls back near the start, fastest at the edge", () => {
    expect(edgeSpeed(0, 0, 1000)).toBe(-18);
    expect(edgeSpeed(28, 0, 1000)).toBe(-9);
  });
  it("scrolls forward near the end", () => {
    expect(edgeSpeed(1000, 0, 1000)).toBe(18);
  });
  it("caps speed when the pointer leaves the container", () => {
    expect(edgeSpeed(1200, 0, 1000)).toBe(18);
  });
});
