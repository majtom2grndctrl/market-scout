import type { RefCallback } from "react";

import type { Drive } from "./drive";
import type { Edge } from "./edges";
import { sameSpan, type Span } from "./light";

/** The role holding the light, and the skills it lights. Changes only when the role does. */
export interface Light {
  readonly role: string;
  /** Lit skills, each with the seconds until its line arrives. */
  readonly lit: ReadonlyMap<string, number>;
}

export interface Line {
  readonly slug: string;
  /** Length when the line appeared, so it draws at the shared speed. */
  readonly length: number;
  /** The edge marker it lands on, when its skill is scrolled out of the column. */
  readonly edge: Edge | null;
}

/**
 * Which lines exist and whether they show. Changes when a role lights, a line
 * appears or drops, or the bundle fades. Line geometry is not here: it
 * changes every scroll frame, so the hook writes it straight to the DOM
 * (see `Bind`).
 */
export interface LineSet {
  readonly role: string;
  /** Lines in port order. Null when the layout is stacked: skills still light, but nothing is drawn. */
  readonly lines: readonly Line[] | null;
  /** False once too little of the role shows between the header and the pin bar to hold its port. */
  readonly visible: boolean;
  /** Fade mode: true while the page scrolls, when no line shows. */
  readonly quiet: boolean;
  /** Fade mode: counts redraws, so each one remounts the bundle and draws it in again. */
  readonly epoch: number;
  /** Compositor mode: the draw-in has finished and the compositor's pieces have taken over. */
  readonly drawn: boolean;
  /** Where the thread's gradient runs. Moves only with the layout. */
  readonly span: Span | null;
  /** Lit skills beyond each of the column's edges: what each edge marker counts. */
  readonly edges: { readonly above: number; readonly below: number };
}

/**
 * Holds `LineSet` outside React state, so a change mid-scroll re-renders the
 * connectors alone (through useSyncExternalStore), not the whole page.
 */
export interface LineStore {
  readonly get: () => LineSet | null;
  readonly subscribe: (onChange: () => void) => () => void;
}

/** Ref for an element whose geometry the hook writes. A drive also hands it a scroll-driven animation. */
export type Bind = (key: string, drive?: Drive) => RefCallback<Element>;

export const lineKey = (role: string, slug: string) => `${role}/${slug}`;
export const portKey = (role: string) => `${role}#port`;
/** The draw-in path's frame, in compositor mode. */
export const frameKey = (role: string) => `${role}#frame`;
/** One compositor piece of a line: `port`, `mark`, or a run's clip, unclip, or bar. */
export const pieceKey = (role: string, slug: string, part: string) => `${role}/${slug}~${part}`;

export type Attrs = Readonly<Record<string, string>>;

export function paint(el: Element, attrs: Attrs) {
  for (const name in attrs) if (el.getAttribute(name) !== attrs[name]) el.setAttribute(name, attrs[name]);
}

export function createStore() {
  let value: LineSet | null = null;
  const subs = new Set<() => void>();
  return {
    get: () => value,
    set: (next: LineSet | null) => {
      value = next;
      subs.forEach((f) => f());
    },
    subscribe: (f: () => void) => {
      subs.add(f);
      return () => void subs.delete(f);
    },
  };
}

export const sameLineSet = (a: LineSet | null, b: LineSet | null) =>
  a === b ||
  (a !== null &&
    b !== null &&
    a.role === b.role &&
    a.visible === b.visible &&
    a.quiet === b.quiet &&
    a.epoch === b.epoch &&
    a.drawn === b.drawn &&
    sameSpan(a.span, b.span) &&
    a.edges.above === b.edges.above &&
    a.edges.below === b.edges.below &&
    (a.lines === b.lines ||
      (a.lines !== null && b.lines !== null && a.lines.length === b.lines.length && a.lines.every((l, i) => l.slug === b.lines?.[i].slug && l.edge === b.lines?.[i]?.edge))));
