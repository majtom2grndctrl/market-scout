"use client";

import "./constellation.css";

import { MotionConfig, motion, useReducedMotion } from "motion/react";
import { useEffect, useMemo, useState } from "react";

import type { DiscoveryData } from "../_data/query";
import { EASE_OUT, Tempo, settleDelay } from "./choreography";
import { countWord, listJoin, plural } from "./copy";
import { WHOLE_KEY, groupKeyOf } from "./geometry";
import { Orientation } from "./orientation";
import { PinTray } from "./pin-tray";
import { RoleDetail } from "./role-detail";
import { SkyPanel } from "./sky-panel";
import type { Focus } from "./star-map";

interface Props {
  data: DiscoveryData;
  initialSelect: string | null;
}

export function Constellation({ data, initialSelect }: Props) {
  const recs = data.recommendations;
  const [pinned, setPinned] = useState<ReadonlySet<string>>(() => new Set(recs.filter((r) => r.pinned).map((r) => r.roleSlug)));
  const [focus, setFocus] = useState<Focus>(() =>
    initialSelect && recs.some((r) => r.roleSlug === initialSelect) ? { kind: "role", slug: initialSelect } : null,
  );
  const [continued, setContinued] = useState(false);
  const [entered, setEntered] = useState(false);

  const tempo = useReducedMotion() ? 0 : 1;
  const settle = tempo * settleDelay(recs.length);
  useEffect(() => {
    const t = window.setTimeout(() => setEntered(true), settle * 1000 + 800);
    return () => window.clearTimeout(t);
  }, [settle]);

  const togglePin = (slug: string) => {
    setContinued(false);
    setPinned((prev) => {
      const next = new Set(prev);
      if (next.has(slug)) next.delete(slug);
      else next.add(slug);
      return next;
    });
  };

  const selected = focus?.kind === "role" ? recs.find((r) => r.roleSlug === focus.slug) : undefined;
  const pinnedRecs = useMemo(() => recs.filter((r) => pinned.has(r.roleSlug)), [recs, pinned]);
  const claimedCount = data.claimedSkills.length;

  return (
    <MotionConfig reducedMotion="user">
      <Tempo.Provider value={tempo}>
      <main className="constellation-root mx-auto flex w-full max-w-[1480px] flex-col gap-5 px-6 pt-7 pb-12">
        <Intro data={data} />

        <div
          className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]"
          onKeyDown={(e) => {
            if (e.key === "Escape") setFocus(null);
          }}
        >
          <SkyPanel data={data} pinned={pinned} focus={focus} entered={entered} onFocus={setFocus} onTogglePin={togglePin} />

          {/* The rail matches the map's height: the detail scrolls inside it, so
              the tray and its continue action never fall below the map. */}
          <aside className="relative">
            <div className="flex flex-col gap-4 xl:absolute xl:inset-0">
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: tempo * 0.4, duration: tempo * 0.6 }}
                className="min-h-0 flex-1 overflow-y-auto rounded-xl border border-edge-hairline bg-surface-raised p-5 shadow-sm"
              >
                {selected ? (
                  <RoleDetail
                    rec={selected}
                    pinned={pinned.has(selected.roleSlug)}
                    claimedCount={claimedCount}
                    onTogglePin={() => togglePin(selected.roleSlug)}
                  />
                ) : (
                  <Orientation
                    data={data}
                    activeGroup={focus?.kind === "group" ? focus.key : null}
                    onGroup={(key) => setFocus(key ? { kind: "group", key } : null)}
                  />
                )}
              </motion.div>

              <PinTray
                className="shrink-0"
                pinned={pinnedRecs}
                delay={settle}
                continued={continued}
                onUnpin={togglePin}
                onFocus={(slug) => setFocus({ kind: "role", slug })}
                onContinue={() => setContinued(true)}
              />
            </div>
          </aside>
        </div>
      </main>
      </Tempo.Provider>
    </MotionConfig>
  );
}

function Intro({ data }: { data: DiscoveryData }) {
  const titles = [...new Set(data.pastRoles.map((p) => p.titleText))];
  const n = data.recommendations.length;
  const claimed = data.claimedSkills.length;

  // The title most roles build on, cited by name when it carries more than one.
  const counts = new Map<string, number>();
  for (const r of data.recommendations) counts.set(groupKeyOf(r), (counts.get(groupKeyOf(r)) ?? 0) + 1);
  const [leadKey, leadCount] = [...counts.entries()].filter(([k]) => k !== WHOLE_KEY).sort((a, b) => b[1] - a[1])[0] ?? [null, 0];

  const centre =
    titles.length > 0
      ? `${listJoin(titles)} ${titles.length === 1 ? "sits" : "sit"} at the centre.`
      : `Your ${countWord(claimed)} claimed ${plural(claimed, "skill")} sit at the centre.`;
  const orbit =
    n > 0
      ? ` The ${countWord(n)} ${plural(n, "role")} around ${titles.length === 1 ? "it" : "them"} are ranked against ${titles.length > 0 ? `${titles.length === 1 ? "that title" : "those titles"} and ` : ""}your ${countWord(claimed)} claimed ${plural(claimed, "skill")}. The nearer the orbit, the higher the rank.`
      : " No roles are ranked for this profile yet.";

  return (
    <motion.header
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.6, ease: EASE_OUT }}
      className="max-w-[48rem] space-y-2.5"
    >
      <p className="text-[11px] font-semibold tracking-[0.16em] text-content-muted uppercase">Discovery</p>
      <h1 className="font-display text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance">
        Here&rsquo;s where you stand, and what&rsquo;s within reach.
      </h1>
      <p className="text-[15px] leading-relaxed text-pretty text-content-secondary">
        {centre}
        {orbit}
        {leadKey && leadCount > 1 && (
          <>
            {" "}
            <span className="text-content-primary">
              {leadKey} alone leads to {countWord(leadCount)} of them.
            </span>
          </>
        )}
      </p>
    </motion.header>
  );
}
