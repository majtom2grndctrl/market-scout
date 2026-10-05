"use client";

import { LayoutGroup, MotionConfig, motion } from "motion/react";
import { useCallback, useMemo, useState } from "react";

import type { DiscoveryData, Recommendation } from "../../_data/query";
import styles from "../airy.module.css";
import { ArrivalClock } from "../_lib/arrival";
import { tiersOf } from "../_lib/copy";
import { DRIFT } from "../_lib/motion";
import { Hero } from "./hero";
import { PinTray } from "./pin-tray";
import { TierSection } from "./tier-section";
import { cn } from "@/lib/utils";

export function AiryDiscovery({ data }: { data: DiscoveryData }) {
  // Pins are local by contract: seeded from the profile, never written back.
  // Kept in the order they were made, so the tray shows the newest.
  const [order, setOrder] = useState<string[]>(() =>
    data.recommendations.filter((r) => r.pinned).map((r) => r.roleSlug),
  );
  const [acknowledged, setAcknowledged] = useState(false);
  const [startedAt] = useState(() => performance.now());

  const toggle = useCallback((roleSlug: string) => {
    setAcknowledged(false);
    setOrder((o) => (o.includes(roleSlug) ? o.filter((s) => s !== roleSlug) : [...o, roleSlug]));
  }, []);

  const pinned = useMemo(() => new Set(order), [order]);
  const pins = useMemo(() => {
    const bySlug = new Map(data.recommendations.map((r) => [r.roleSlug, r]));
    return order.flatMap((s): Recommendation[] => {
      const r = bySlug.get(s);
      return r ? [r] : [];
    });
  }, [order, data.recommendations]);
  const tiers = useMemo(() => tiersOf(data.recommendations), [data.recommendations]);

  return (
    <MotionConfig reducedMotion="user">
      <ArrivalClock.Provider value={startedAt}>
        <LayoutGroup id="airy">
          <div className={cn(styles.root, "relative isolate min-h-[calc(100svh-3.5rem)] pb-8")}>
            <motion.div
              aria-hidden
              className={cn(styles.sky, "pointer-events-none absolute inset-x-0 top-0 -z-10 h-[58rem]")}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ duration: 2.2, ease: DRIFT }}
            />

            <div className="mx-auto max-w-[76rem] px-6 md:px-10 lg:px-14">
              <Hero data={data} />
              <div className="mt-20 space-y-28 md:mt-24">
                {tiers.map((t, i) => (
                  <TierSection
                    key={t.copy.strength}
                    copy={t.copy}
                    recs={t.recs}
                    index={i}
                    pinned={pinned}
                    onToggle={toggle}
                  />
                ))}
              </div>
            </div>

            <PinTray pins={pins} onUnpin={toggle} acknowledged={acknowledged} onContinue={() => setAcknowledged(true)} />
          </div>
        </LayoutGroup>
      </ArrivalClock.Provider>
    </MotionConfig>
  );
}
