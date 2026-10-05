// Prototype — asked 2026-10-05.
//
// Question: can Discovery lift someone at their lowest point through calm
// alone? Airy answers with light and room: a dawn sky, the person's past read
// back as evidence, and roles drifting up nearest-first, each tier given less
// space as it sits further out. A pin flies to a floating tray, where the way
// on to the dashboard waits.
//
// Data and invariants: `../_data/query.ts` and
// agent-context/plans/in-progress/discovery-prototypes/index.md.

import { getDiscoveryData } from "../_data/query";
import { AiryDiscovery } from "./_components/airy-discovery";

export default async function AiryPage() {
  const data = await getDiscoveryData();
  return <AiryDiscovery data={data} />;
}
