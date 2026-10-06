// Prototype — asked 2026-10-05.
//
// Question: does Discovery land harder when it shows the person their own
// skills first, then how each recommended role draws on them? Threads lists
// the skills they named and the ones that come with roles they've held, and
// lets a role light the skills it draws on, with connector lines from role to
// skill. The story: the skills are already there; you hadn't seen how they
// come together.
//
// Data and invariants: `../_data/query.ts` and
// agent-context/plans/in-progress/discovery-prototypes/index.md.
//
// `?lines=compositor|fade|js` picks how the lines behave while the page
// scrolls (see `_lib/modes.ts`); compositor is the default.

import { getDiscoveryData } from "../_data/query";
import { ThreadsDiscovery } from "./_components/threads-discovery";
import { parseLineMode } from "./_lib/modes";

export default async function ThreadsPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const [data, params] = await Promise.all([getDiscoveryData(), searchParams]);
  return <ThreadsDiscovery data={data} lines={parseLineMode(params.lines)} />;
}
