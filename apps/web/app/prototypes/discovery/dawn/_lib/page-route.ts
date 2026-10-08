// Lines routed whole on the main thread, in the container's frame: the `fade`
// and `js` modes (see `modes.ts`). Pure: this pass's measurements in, the
// line set and element attributes out.

import type { Recommendation } from "../../_data/query";
import { reachOf } from "./compose";
import { endsOf, type Routing } from "./edges";
import { routeBundle, type Route, type Target } from "./geometry";
import { HEADER, type Layout } from "./layout";
import type { Span } from "./light";
import { lineKey, portKey, type Attrs, type LineSet } from "./store";

export interface PageRoute {
  readonly next: LineSet;
  readonly routes: readonly Route[];
  readonly attrs: ReadonlyMap<string, Attrs>;
}

export function routeOnPage(
  active: Recommendation,
  L: Layout,
  routing: Routing,
  /** The container's and the column's boxes in the viewport, and the pin bar's top. */
  box: DOMRect,
  col: DOMRect,
  barTop: number,
  scrollTop: number,
  fade: { readonly hush: boolean; readonly epoch: number },
  span: Span,
): PageRoute {
  // A skill in the column's band takes its line on its own marker, which
  // scrolls with the column; one beyond an edge takes it on that edge's
  // marker, which does not.
  const ends = endsOf(L, routing);
  const targets = ends.map((e): Target => ({ slug: e.slug, x: e.x, y: col.top + e.y - (e.edge ? 0 : scrollTop) - box.top, width: e.width }));
  const edgeOf = new Map(ends.map((e) => [e.slug, e.edge]));

  // The port sits level with the role's headline, and slides along the
  // role's visible edge as it scrolls under the header or the pin bar, so
  // the bundle leaves from what the reader can still see of the role.
  const reach = reachOf(targets.map((t) => t.width ?? 1.5));
  const floor = Math.min(L.viewH, barTop);
  let visible = false;
  let originY = box.top;
  if (L.anchor) {
    const lo = Math.max(box.top + L.anchor.top, HEADER) + reach;
    const hi = Math.min(box.top + L.anchor.bottom, floor) - reach;
    visible = lo <= hi;
    originY = Math.min(Math.max(box.top + L.anchor.mid, lo), hi);
  }

  const bundle = routeBundle({ x: L.originX, y: originY - box.top }, L.gutterLeft, targets);
  const attrs = new Map<string, Attrs>();
  attrs.set(portKey(active.roleSlug), {
    x: String(L.originX - 1.5),
    y: String(bundle.portTop - 7),
    height: String(bundle.portBottom - bundle.portTop + 14),
  });
  for (const r of bundle.routes) attrs.set(lineKey(active.roleSlug, r.slug), { d: r.d });

  return {
    next: {
      role: active.roleSlug,
      lines: bundle.routes.map((r) => ({ slug: r.slug, length: r.length, edge: edgeOf.get(r.slug) ?? null, width: r.target.width ?? 1.5 })),
      visible,
      quiet: fade.hush,
      epoch: fade.epoch,
      drawn: true,
      span,
      edges: { above: routing.above.length, below: routing.below.length },
    },
    routes: bundle.routes,
    attrs,
  };
}
