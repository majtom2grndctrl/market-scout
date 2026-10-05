// Prototype — asked 2026-10-05.
//
// Question: can Discovery lift a person who is unsure where they fit by
// speaking to them directly, one idea per screen, from the titles they have
// held to the roles worth pinning? One of five sketches over the same read;
// see agent-context/plans/in-progress/discovery-prototypes/index.md.
//
// `?beat=<n>` opens on the nth screen, so every beat is reviewable headless.

import { getDiscoveryData } from "../_data/query";
import { composeLetter } from "./copy";
import { Letter } from "./letter";

export default async function LetterPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const [data, params] = await Promise.all([getDiscoveryData(), searchParams]);
  const requested = Number(Array.isArray(params.beat) ? params.beat[0] : params.beat);

  // The letter is composed here, so the score never reaches the client.
  return (
    <Letter
      beats={composeLetter(data)}
      initialBeat={Number.isInteger(requested) ? requested - 1 : 0}
      initialPins={data.recommendations.filter((r) => r.pinned).map((r) => r.roleSlug)}
    />
  );
}
