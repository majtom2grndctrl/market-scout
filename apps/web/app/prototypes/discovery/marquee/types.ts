import type { DiscoveryData, Recommendation } from "../_data/query";

/** A recommendation with its internal score stripped before it reaches the client. */
export type Role = Omit<Recommendation, "score">;

export type MarqueeData = Omit<DiscoveryData, "recommendations"> & {
  readonly recommendations: readonly Role[];
};
