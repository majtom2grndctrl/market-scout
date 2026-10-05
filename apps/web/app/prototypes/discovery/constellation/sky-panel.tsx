"use client";

import { motion } from "motion/react";
import { useLayoutEffect, useMemo } from "react";

import type { DiscoveryData, Strength } from "../_data/query";
import { useTempo } from "./choreography";
import { ORBIT_NAME, postings } from "./copy";
import { layoutConstellation } from "./geometry";
import { PinGlyph } from "./pin-glyph";
import { placeOrbitLabels, placeStarLabels } from "./labels";
import { type Focus, StarMap } from "./star-map";
import { useChartFrame } from "./use-chart-frame";

interface Props {
  data: DiscoveryData;
  pinned: ReadonlySet<string>;
  focus: Focus;
  entered: boolean;
  onFocus: (focus: Focus) => void;
  onTogglePin: (slug: string) => void;
}

// Below this the map scrolls sideways inside its panel rather than crushing
// twelve labels into a phone width.
const MIN_WIDTH = 660;
// The legend strip under the map, plus breathing room above the fold.
const FOOTER = 64;

export function SkyPanel({ data, pinned, focus, entered, onFocus, onTogglePin }: Props) {
  const { ref, width: frameWidth, room, measure } = useChartFrame();
  const k = useTempo();
  const width = Math.max(frameWidth, MIN_WIDTH);
  // Fit the first screen when it can: the whole figure should be seen at once.
  const height = Math.round(Math.min(800, Math.max(600, Math.min(width * 0.9, room - FOOTER))));

  const recs = useMemo(() => new Map(data.recommendations.map((r) => [r.roleSlug, r])), [data.recommendations]);

  const chart = useMemo(() => {
    if (!measure || frameWidth === 0) return null;
    const layout = layoutConstellation(data, width, height, measure);
    // Reserve the pinned form of the meta line, so pinning never moves a label.
    const labels = placeStarLabels(
      layout,
      (slug) => {
        const r = recs.get(slug);
        return { headline: r?.headline ?? slug, meta: `Pinned · ${postings(r?.openPostings ?? 0)}` };
      },
      measure,
    );
    const orbitLabels = placeOrbitLabels(layout, labels, measure);
    return { layout, labels, orbitLabels };
  }, [data, width, height, measure, frameWidth, recs]);

  // When the map is wider than its panel, open on the centre, where the person stands.
  useLayoutEffect(() => {
    const el = ref.current;
    if (el && chart && el.scrollWidth > el.clientWidth) el.scrollLeft = (el.scrollWidth - el.clientWidth) / 2;
  }, [chart, ref]);

  const tiers = (["close", "adjacent", "stretch"] as const).filter((t) => data.recommendations.some((r) => r.strength === t));
  const { coverage } = data;

  return (
    <div className="constellation-sky dark relative overflow-hidden rounded-2xl">
      <motion.p
        className="px-5 pt-4 text-[11px] text-content-muted"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: k * 2.6, duration: k * 0.8 }}
      >
        Hover or Tab to a star to read it. Click or press Enter to pin it.
      </motion.p>
      <div ref={ref} className="overflow-x-auto" style={{ minHeight: height }}>
        {chart && (
          <StarMap
            layout={chart.layout}
            labels={chart.labels}
            orbitLabels={chart.orbitLabels}
            recs={recs}
            pinned={pinned}
            focus={focus}
            entered={entered}
            onFocus={onFocus}
            onTogglePin={onTogglePin}
          />
        )}
      </div>

      <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 border-t border-edge-hairline/60 px-5 py-3 text-[11px] text-content-muted">
        <p>
          Ranked from {coverage.classifiedPostings.toLocaleString("en-US")} classified of {coverage.openPostings.toLocaleString("en-US")} open
          postings · {coverage.rolesConsidered} roles considered
        </p>
        {tiers.length > 0 && (
          <ul className="flex items-center gap-4" aria-label="Legend">
            {tiers.map((t) => (
              <li key={t} className="flex items-center gap-1.5">
                <TierDot strength={t} />
                {ORBIT_NAME[t]}
              </li>
            ))}
            <li className="flex items-center gap-1.5">
              <PinGlyph className="text-(--constellation-pin)" />
              Pinned
            </li>
          </ul>
        )}
      </div>
    </div>
  );
}

function TierDot({ strength }: { strength: Strength }) {
  const r = { close: 4, adjacent: 3.1, stretch: 2.4 }[strength];
  return (
    <svg width={10} height={10} viewBox="-5 -5 10 10" aria-hidden>
      <circle r={r} fill="var(--constellation-star)" />
    </svg>
  );
}
