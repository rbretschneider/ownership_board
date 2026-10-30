/** Bounds of the autonomous in-flight cap; the server enforces the same. */
export const MIN_IN_FLIGHT_CAP = 1;
export const MAX_IN_FLIGHT_CAP = 10;

/** The values offered for the in-flight cap, smallest first. */
export const IN_FLIGHT_CAP_CHOICES: readonly number[] = Array.from(
  { length: MAX_IN_FLIGHT_CAP - MIN_IN_FLIGHT_CAP + 1 },
  (_, i) => MIN_IN_FLIGHT_CAP + i,
);
