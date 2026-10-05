// Prototype — asked 2026-10-05.
//
// Question: can display typography alone carry Discovery? Marquee sets the
// person's past as one enormous sentence, then gives each recommended role a
// poster: its headline sized to fill the measure, the other titles it goes by
// rolling beneath, and the reasons set as type rather than chips.
//
// Data comes only from `getDiscoveryData()`. The score is stripped here, so it
// never reaches the client at all.

import "./marquee.css";

import { getDiscoveryData } from "../_data/query";
import { Marquee } from "./marquee";
import type { MarqueeData } from "./types";

export default async function MarqueePage() {
  const data = await getDiscoveryData();
  const scrubbed: MarqueeData = {
    ...data,
    recommendations: data.recommendations.map(({ score: _score, ...role }) => role),
  };
  return <Marquee data={scrubbed} />;
}
