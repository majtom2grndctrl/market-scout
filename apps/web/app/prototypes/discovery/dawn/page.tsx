// Prototype — asked 2026-10-05.
//
// Question: does Threads' story land harder lit as first light? Dawn keeps
// Threads' layout, data, and lines (skills left, roles right, a role lighting
// the skills it draws on) and re-lights them in Airy's dawn palette: a sky
// held to the viewport that the roles card scrolls across, blue overhead and
// warm at the horizon, the sun rising behind the roles as the first role
// lights, threads that warm from lilac at the skills to rose at the role's
// gold port, and lit skills that catch the light. The page meets someone at
// the low point of a job search; it should feel like morning, earned by what
// they already have.
//
// Data and invariants: `../_data/query.ts` and
// agent-context/plans/in-progress/discovery-prototypes/index.md.
//
// `?lines=compositor|fade|js` picks how the lines behave while the page
// scrolls (see `_lib/modes.ts`); compositor is the default.

import { getDiscoveryData } from "../_data/query";
import { DawnDiscovery } from "./_components/dawn-discovery";
import { parseLineMode } from "./_lib/modes";

export default async function DawnPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const [data, params] = await Promise.all([getDiscoveryData(), searchParams]);
  return <DawnDiscovery data={data} lines={parseLineMode(params.lines)} />;
}
