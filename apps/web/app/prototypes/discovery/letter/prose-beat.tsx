"use client";

import type { ProseBeat } from "./copy";
import { joinList } from "./grammar";
import { Arrive, phrases, timeline } from "./reveal";
import styles from "./letter.module.css";

export function ProseBeatView({ beat }: { beat: ProseBeat }) {
  const tl = timeline();
  const mark = beat.footnote ? <sup className={styles.footnoteMark}>1</sup> : null;

  // Cues are taken in reading order, top to bottom: the order of these
  // statements is the choreography.
  const display = phrases(beat.display, tl, { first: 0.28, note: mark });
  const body = beat.body.map((line, i) => phrases(line, tl, { first: i === 0 ? 0.55 : 0.3 }));
  const rosterAt = beat.roster && beat.roster.length > 0 ? tl.cue(0.3) : null;
  const postscriptAt = beat.postscript ? tl.cue(0.9) : null;
  const footnoteAt = beat.footnote ? tl.cue(0.5) : null;

  return (
    <div className="flex flex-1 flex-col">
      <div className="flex flex-1 flex-col justify-center pb-[6vh]">
        <h2 className={`${styles.display} max-w-[24ch] font-display text-content-primary`}>{display}</h2>

        <div className={`${styles.prose} mt-10 max-w-[36rem] space-y-4 text-content-secondary`}>
          {body.map((line, i) => (
            <p key={i}>{line}</p>
          ))}
          {rosterAt !== null && beat.roster ? (
            <Arrive delay={rosterAt} as="p" className="text-content-primary">
              {joinList(beat.roster)}.
            </Arrive>
          ) : null}
        </div>

        {postscriptAt !== null && beat.postscript ? (
          <Arrive delay={postscriptAt} as="p" className="mt-8 max-w-[34rem] border-l-2 border-edge pl-4 text-base leading-relaxed text-pretty text-content-muted">
            {beat.postscript}
          </Arrive>
        ) : null}
      </div>

      {footnoteAt !== null && beat.footnote ? (
        // The note's own mark sits in line at full size: a superscript at
        // footnote scale would fall under the 14px floor.
        <Arrive delay={footnoteAt} as="p" className="flex max-w-[40rem] gap-3 pb-14 text-sm leading-relaxed text-content-muted">
          <span className="font-medium text-(--letter-accent) tabular-nums">1</span>
          <span>{beat.footnote}</span>
        </Arrive>
      ) : null}
    </div>
  );
}
