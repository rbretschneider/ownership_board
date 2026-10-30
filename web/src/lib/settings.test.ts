import { describe, expect, it } from "vitest";
import { IN_FLIGHT_CAP_CHOICES } from "./settings";

describe("IN_FLIGHT_CAP_CHOICES", () => {
  it("offers every cap from 1 to 10 in order", () => {
    expect(IN_FLIGHT_CAP_CHOICES).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
  });
});
