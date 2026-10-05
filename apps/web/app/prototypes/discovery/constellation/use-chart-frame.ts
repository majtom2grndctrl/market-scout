"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";

import { type FontRole, type Measure, TYPE } from "./geometry";

/**
 * Width of the map's container, the first-screen height below it, and a text
 * measurer, all client-only. The
 * layout waits for the fonts: label collision depends on real glyph widths,
 * and a fallback-font measurement would place every label a little wrong.
 */
export function useChartFrame() {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [room, setRoom] = useState(0);
  const [fontsReady, setFontsReady] = useState(false);

  useEffect(() => {
    let live = true;
    Promise.all(Object.values(TYPE).map((t) => document.fonts.load(t.font)))
      .catch(() => undefined)
      .then(() => live && setFontsReady(true));
    return () => {
      live = false;
    };
  }, []);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(Math.round(el.getBoundingClientRect().width));
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setWidth(Math.round(entry.contentRect.width));
    });
    observer.observe(el);
    // Height left in the first screen below the map's top edge.
    const onResize = () => setRoom(Math.round(window.innerHeight - (el.getBoundingClientRect().top + window.scrollY)));
    onResize();
    window.addEventListener("resize", onResize);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", onResize);
    };
  }, []);

  const measure = useMemo(() => (fontsReady ? createMeasure() : null), [fontsReady]);

  return { ref, width, room, measure };
}

function createMeasure(): Measure {
  const ctx = document.createElement("canvas").getContext("2d");
  const cache = new Map<string, number>();
  return (text: string, role: FontRole) => {
    const key = `${role}\u0000${text}`;
    const hit = cache.get(key);
    if (hit !== undefined) return hit;
    const t = TYPE[role];
    let width = text.length * t.size * 0.54;
    if (ctx) {
      ctx.font = t.font;
      width = ctx.measureText(text).width;
    }
    width += text.length * t.tracking * t.size;
    cache.set(key, width);
    return width;
  };
}
