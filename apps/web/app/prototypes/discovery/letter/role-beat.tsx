"use client";

import type { RoleBeat } from "./copy";
import { PinButton } from "./pin-button";
import { Arrive, phrases, timeline, Words } from "./reveal";
import styles from "./letter.module.css";

export function RoleBeatView({ beat, pinned, onTogglePin }: { beat: RoleBeat; pinned: boolean; onTogglePin: () => void }) {
  const tl = timeline();

  // Reading order is cue order: where they come from, the role, what else
  // employers call it, how much of it there is, and last, what they bring.
  const eyebrowAt = tl.cue(0);
  const kickerAt = beat.kicker ? tl.words(beat.kicker, 0.25) : null;
  const headlineAt = tl.words(beat.rec.headline, beat.kicker ? 0.2 : 0.3);
  const alsoAt = beat.alsoCalled ? tl.cue(0.55) : null;
  const echoAt = beat.echo ? tl.words(beat.echo, 0.35) : null;
  const evidence = phrases(beat.evidence, tl, { first: 0.45 });
  const growAt = beat.grow.length > 0 ? tl.cue(0.2) : null;
  const bring = beat.bring ? phrases(beat.bring, tl, { first: 0.5 }) : null;
  const pinAt = tl.cue(beat.bring ? 0.9 : 0.5);

  return (
    <div className="grid flex-1 items-center gap-x-16 gap-y-12 pt-8 pb-[6vh] lg:grid-cols-[minmax(0,1fr)_15rem]">
      <div>
        <Arrive delay={eyebrowAt} as="p" className={styles.eyebrow}>
          {beat.eyebrow}
        </Arrive>

        {kickerAt !== null && beat.kicker ? (
          <p className={`${styles.prose} mt-8 text-content-muted`}>
            <Words text={beat.kicker} delay={kickerAt} />
          </p>
        ) : null}

        <h2 className={`${styles.headline} ${beat.kicker ? "mt-2" : "mt-8"} max-w-[14ch] font-display text-content-primary`}>
          <Words text={beat.rec.headline} delay={headlineAt} />
        </h2>

        {alsoAt !== null && beat.alsoCalled ? (
          <Arrive delay={alsoAt} as="p" className="mt-6 max-w-[40rem] text-base leading-relaxed text-pretty text-content-muted">
            {beat.alsoCalled}
            {echoAt !== null && beat.echo ? (
              <>
                {" "}
                <span className="text-content-secondary">
                  <Words text={beat.echo} delay={echoAt} />
                </span>
              </>
            ) : null}
          </Arrive>
        ) : null}

        <p className={`${styles.prose} mt-10 text-content-secondary`}>{evidence}</p>

        {bring ? <p className={`${styles.display} mt-3 max-w-[22ch] font-display text-content-primary`}>{bring}</p> : null}

        <Arrive delay={pinAt} className="mt-10">
          <PinButton pinned={pinned} onToggle={onTogglePin} name={beat.rec.headline} />
        </Arrive>
      </div>

      {growAt !== null ? (
        <Arrive delay={growAt} as="aside" className="border-t border-edge pt-5 lg:mt-24 lg:self-end lg:border-t-0 lg:border-l lg:pt-0 lg:pl-6">
          <h3 className="text-xs font-medium tracking-[0.12em] text-content-muted uppercase">Its postings also ask for</h3>
          <ul className="mt-4 space-y-3">
            {beat.grow.map((g) => (
              <li key={g.name} className="text-sm leading-snug">
                <span className="block text-content-primary">{g.name}</span>
                <span className="text-content-muted tabular-nums">in {g.share} of postings</span>
              </li>
            ))}
          </ul>
        </Arrive>
      ) : null}
    </div>
  );
}
