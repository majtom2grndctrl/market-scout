// Prototype — asked 2026-10-05.
//
// Question: does Discovery lift a person who is unsure where they fit when it
// answers spatially — "you are not lost; here is where you stand, and here is
// what is within reach"? Past titles sit at the centre. Recommended roles
// orbit them, nearer for a higher rank, each fanning out from the past title
// it builds on. One of five Discovery sketches over the same read; see
// agent-context/plans/in-progress/discovery-prototypes/index.md.
//
// Knob: `?select=<roleSlug>` opens a role's detail on load, so the selected
// state can be reviewed without hovering.

import { getDiscoveryData } from "../_data/query";
import { Constellation } from "./constellation";

export default async function ConstellationPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const [data, params] = await Promise.all([getDiscoveryData(), searchParams]);
  const select = typeof params.select === "string" ? params.select : null;
  return <Constellation data={data} initialSelect={select} />;
}
