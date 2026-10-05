// Prototype — asked 2026-10-05.
//
// Question: can Discovery lift a person through clarity rather than warmth?
// Ledger sets the recommendation as a ranked table, every reason in its own
// column, so "here is exactly where you stand" reads as evidence. One of five
// sketches over `../_data/query.ts`; see the discovery-prototypes plan.

import { getDiscoveryData } from "../_data/query";
import { Ledger } from "./ledger";

export default async function LedgerPage() {
  const data = await getDiscoveryData();
  return <Ledger data={data} />;
}
