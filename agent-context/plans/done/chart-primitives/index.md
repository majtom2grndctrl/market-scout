# Chart Primitives

> Draft — decisions, tasks, acceptance criteria. Roadmap: Compositional Analysis Surface → Chart.
> Surface: web
> Consumes: `measure-engine` (`MeasureResult`) and `composition-grammar` (the `Encoding` union / `ENCODINGS` const array, `Grouping`/`Measure`/`Cohort` types). Both are the source of truth; this spec renders their output and never redefines their vocabulary.

## Goal

The chart layer of the Compositional Analysis Surface: render a `MeasureResult` into SVG for every one of the seven grammar encodings, honestly. It is the third leg of the grammar's boundary — the grammar validates, the engine computes numbers, this layer draws them. Its signature obligation is drawing *absence* as absence: a failed-run week breaks the line rather than smoothing across a value no run backs. It ships as an app-owned primitive set (d3 supplies math, the app owns every mark) plus a Storybook gallery that renders all seven encodings — including the honest degenerate cases — from fixtures, proving the foundation before any route or transport binds to it.

## Target usage

```tsx
// Proposed design — remove after implementation.

// Layer 1 — shaping (PURE, SSR-safe: no `postgres` import, no window/document/ResizeObserver).
// MeasureResult → plottable series. Computes representation only, never a source-of-truth number.
import { shapeForEncoding } from "@/lib/chart/shaping";
const plotted = shapeForEncoding(result); // bins histograms, stacks shares, splits gap segments

// Layer 2 — the composition layer constructs all scales + margins + viewBox and passes finished
// scales to a per-encoding chart component; the encoding component builds no scale.
// Coverage is a discriminated union over result type: the classified arm REQUIRES `coverage`; the full-corpus arm omits it.
<CompositionChart result={classifiedResult} coverage={{ classified: 1842, total: 6969 }} />;
<CompositionChart result={fullCorpusResult} />;
// A statically-typed classified composition WITHOUT coverage is a COMPILE error:
//   <CompositionChart result={shareBySeniority} />          // ✗ coverage required for a classified result
// A model-emitted (dynamic) classified result with no coverage is REFUSED at runtime — no naked slice is drawn.

// Layer 3 — primitives take scales as PROPS; they never build a scale.
<Bars xScale={x} yScale={y} data={plotted.bars} className="fill-primary" />  // each datum carries a variant flag
<Line xScale={x} yScale={y} segments={plotted.segments} />  // one path per pre-split segment
<GapBand xScale={x} region={plotted.gapRegions} />          // shaded no-data band + annotation
```

## Decisions

- **d3 as math, app owns marks.** `d3-scale`, `d3-shape`, `d3-array`, `d3-time`, `d3-time-format` (already in `apps/web/package.json`) supply scales and path/arc geometry only. Every mark is an app-owned React component emitting plain SVG JSX (`<rect> <path> <text>`) carrying Tailwind classes. No charting component library, and no d3 owning the DOM — no `d3.axisBottom()`, no `selection.append()`. Rationale: the app must own its marks to carry design-system classes and to stay reviewable as JSX, not as imperative d3 side effects.
- **Three layers, one direction.** (1) Dimensionless shaping: `MeasureRow[]` → plottable series (bins, stacks, gap-splits, bar-variant flags); pure and SSR-safe. (2) Composition: constructs every scale and owns margins, axes, viewBox math; UTC time scales so server and client compute identical ticks. (3) Primitives: receive scales as props, never build their own. The layers import downward only (composition imports shaping and primitives; shaping imports neither).
- **Decision 1 — absence is drawn as absence, per encoding family.** A `gap:true` week (`value: 0`, the flag is the signal) breaks any weekly series and is never interpolated, tweened, dropped, or drawn as a real 0. How absence is drawn depends on the encoding family:
  - **Line/area** (`line`, `area`): shaping pre-splits the series into contiguous gap-free segments; the primitive renders one path per segment (a broken line) and a `GapBand` draws a shaded gray no-data region plus the annotation ("gray marks a gap in collected data") over each gap. `line` and `area` both render a real gap band in v1 — not a dormant capability.
  - **Bars** (`ranked_bars`, `diverging_bars`, `stacked_bar`): "broken line + band" is meaningless for bars, and drawing a gap as a real 0 bar would assert a value no run backs. A bar gap week renders instead as a distinct **no-data column** — a gray/hatched placeholder at the axis baseline carrying the same "gray marks a gap in collected data" annotation, the bar-world analogue of the line's `GapBand`. The mechanism is a `variant` flag on the shaped bar datum (see Inter-layer contracts): `Bars` renders a `no-data`-variant datum as a hatched no-data column, keeping the primitive dumb — the flag decides, not primitive logic.

  The engine emits week-grain `gap` rows for the whole count family (`count`, `delta`, `share`) as well as `rate` whenever the grouping includes `week`, so both families must handle absence in v1. Rationale: absence is the product's signature finding, honestly rendered on every weekly series; a smoothed line or a phantom 0 bar asserts a value no successful run backs.
- **Decision 2 — histogram binning uses fixed, declared, genuinely uniform 7-day edges.** The shaping layer bins raw per-posting `age`/`lifespan` values (fractional-day doubles from the engine) into uniform 7-day buckets, half-open `[lo, hi)`. Binning precedence is fixed: `value === 0` matches the separate `{0}` bucket first (the "died the day it opened" spike is a real finding, not noise); every `value > 0` falls in the half-open `[lo, hi)` bin containing it (so `0 < v < 7` → the first `[0, 7)` bin, `3.5` → `[0, 7)`); `value >= 70` folds into the `70+` open top bucket. Above `{0}`, uniform 7-day ranges run to 70 — inner edges `7, 14, 21, 28, 35, 42, 49, 56, 63, 70` — yielding the full bucket set `{0}`, `[0,7)`, `[7,14)`, … `[63,70)`, `70+`. Every 7-day bin is exactly 7 wide; there are no 14-wide bins. `d3.bin()`'s adaptive default is forbidden: adaptive edges differ across renders, break SSR parity, and invent a boundary the chart never chose. Deterministic edges give server/client parity and make the distribution's shape a reviewable decision.
- **Decision 3 — coverage denominator ownership.** The surrounding frame owns the coverage *value* (it reads `result.denominator`); this chart owns rendering the coverage *pill* and enforcing the prop. The mechanism is a discriminated union **over result type**, not an optional `coverage?` (an optional field cannot error on absence). Define `Coverage = { classified: number; total: number }` and `ClassifiedResult`/`FullCorpusResult` as `MeasureResult` narrowed on `denominator` presence; the props are `{ result: FullCorpusResult } | { result: ClassifiedResult; coverage: Coverage }`. So a value **statically typed** as `ClassifiedResult` without `coverage` is a compile error. Runtime backstop: a chart handed a bare dynamic `MeasureResult` (`denominator` optional, the type can't narrow — e.g. model-emitted) whose `denominator` is present but with no `coverage` refuses to render rather than drawing a naked slice. This is the static-compile / dynamic-runtime split: the type stops known-classified compositions, the backstop stops the rest. Boundary: this spec renders the pill from a passed value and stops; the shared coverage chrome/treatment across chat and page is `composition-frame-and-recovery`.
- **Decision 4 — RWD from day one via a measured wrapper.** Responsiveness is progressive enhancement: server-render a fixed-`viewBox` fallback from `chartTheme` (correct aspect, coverage pill and gap band present on first paint), then a client `ResizeObserver` reflows tick density, label rotation, and small-multiple panels-per-row to the real width. Consequence, stated plainly: charts are **client** components (`ResizeObserver` is browser-only). This trades pure RSC token streaming for adaptive responsiveness while keeping server-rendered first paint — a client component still SSRs its initial HTML — so there is no blank flash and no hydration mismatch (deterministic fallback + UTC scales guarantee identical server/client geometry).
- **Decision 5 — chartTheme is a static plain-JS object.** Chart spacing, margins, gutters, band padding, histogram bin edges, and reflow breakpoints all derive from the app's spacing/breakpoint scale via a shared plain-JS `chartTheme` object, imported statically and overridable by prop. Rationale for the vehicle: spacing feeds d3 *geometry* (JS numbers), so CSS variables can't carry it (not readable as numbers during SSR); a static import avoids the premature client boundary that React Context (`useContext`) would force. Runtime theme-switching via Context is a future option, not v1.
- **Decision 6 — the reading-aid guardrail (v1: prohibition only).** v1 builds **no** drawn reading aid — its only consumer, scatter, is deferred. So v1's rule is a prohibition: shaping fits **no** model (no LOESS, regression, spline) and interpolates across **no** absent point. The *permission* — computing rank statistics (quantiles, medians) as aids that are *drawn, never narrated* — arrives with `scatter-with-trend`, where the drawn aid first has a consumer. The governing test that will bound that permission: a number that gets spoken/claimed (narrated in prose, used as a sort key, encoded in the link) must come from the engine (source of truth, parity); a number only *drawn* as a visual aid may live in shaping. "Summarize the points you were given; never assert a point you weren't." The guardrail belongs here because it governs all shaping, but v1 draws nothing under it.
- **Render order is result order.** The chart never re-sorts. `sort`/`limit` are part of the composition; the engine returns rows already ordered. `ranked_bars`, `diverging_bars`, and `table` render the row order they receive.
- **Proposed file locations.** Pure shaping + theme under `apps/web/lib/chart/` (pure, SSR-safe, no `postgres`, no browser APIs). Marks, per-encoding charts, and the gallery story under `apps/web/components/chart/` (Storybook's glob scans only `components/**`). These are proposals, consistent with `lib/composition/` (grammar) and `lib/db/` (engine); the implementer may rename, but the pure/SSR-safe constraint on `lib/chart/` is not negotiable.

## Input contract (from `measure-engine`)

The chart consumes, and never redefines, this shape (`MeasureResult` from `@/lib/db/measure-engine`; vocabulary types from `@/lib/composition`):

- `MeasureResult` = `{ measure, cohort, groupBy, rows: readonly MeasureRow[], denominator?: { classified: number; total: number } }`.
- `MeasureRow` = `{ keys: Partial<Record<Grouping, string>>; value: number; gap?: boolean }`.
- Engine hands **raw per-posting** values for `age`/`lifespan`; binning is this layer's job (Decision 2).
- `gap: true` rows carry `value: 0` and mark a week with a failed fetch run; the flag is the signal, never the value. The engine emits week-grain gap rows for every weekly series — `count`, `delta`, `share`, and `rate` alike — whenever the grouping includes `week`.
- `share` arrives **pre-normalized globally to sum 1.0**; the chart stacks and scales it, it never re-normalizes.
- `denominator` is present **iff** the grouping/filter is classified (`requiresDenominator`).

## Inter-layer contracts

The shapes shaping (Task 2) emits and the components (Tasks 3–6) consume. Behaviors are pinned elsewhere; these pin the types at each seam. Sketches are proposals — names may change, the shapes are the contract.

```ts
// Proposed design — remove after implementation.

// Coverage prop union — a discriminated union OVER RESULT TYPE, never an optional `coverage?`.
type Coverage = { classified: number; total: number };
type ClassifiedResult = MeasureResult & { denominator: Coverage };   // denominator present
type FullCorpusResult = MeasureResult & { denominator?: never };     // denominator absent
type CoverageProps =
  | { result: FullCorpusResult }
  | { result: ClassifiedResult; coverage: Coverage };

// Shaped bar datum — what `Bars` consumes, one per bar. `variant` carries both `unmapped`
// distinctness and the bar gap column to the dumb primitive via a FIELD, not primitive logic.
type BarVariant = "normal" | "unmapped" | "no-data";  // "no-data" = a weekly gap:true column
type ShapedBar = { key: string; value: number; label: string; variant: BarVariant };

// Line/area segments — shaping PRE-SPLITS the series into contiguous gap-free segments;
// `Line`/`Area` render ONE PATH PER SEGMENT. `.defined()` is at most an internal detail of how
// shaping splits, never something the primitives do on gap-flagged data.
type PlotPoint = { x: Date; y: number };  // x in UTC domain units
type Segment = PlotPoint[];               // contiguous, gap-free
type PlottedSeries = { segments: Segment[]; gapRegions: GapRegion[] };

// Gap region — what `GapBand` consumes: {start, end} in the x-scale's domain units (UTC Date for
// time series), NOT pixels; the primitive maps to pixels through the passed xScale.
type GapRegion = { start: Date; end: Date };

// Stacked cell — the cumulative-offset shape for `stacked_bar`; offsets preserve the global 1.0 sum.
type StackedCell = { key: string; series: string; y0: number; y1: number; variant: BarVariant };

// Histogram panel — the per-panel structure for small multiples. xDomain and yDomain are SHARED
// across every panel (passed once, not recomputed per panel) so panels compare honestly.
type HistogramBin = { x0: number; x1: number | null; count: number; isZeroDay: boolean };  // x1 null = 70+ open top
type HistogramPanel = { group: string; bins: HistogramBin[] };
type HistogramPlot = { panels: HistogramPanel[]; xDomain: number[]; yDomain: [number, number] };

// Reflow-breakpoint entry — an ordered array in `chartTheme` (Task 1); Task 4's ResizeObserver
// picks the entry matching the measured width. Only the numeric thresholds are an Open Question.
type ReflowBreakpoint = {
  minWidth: number;
  tickDensity: number;
  rotateLabels: boolean;
  panelsPerRow: number;
};
```

## Encodings (the seven, from the grammar's `ENCODINGS`)

Render exactly these — no scatter, no radial/pie (deferred, see Not doing).

| Encoding | Measure(s) it renders | Shape / honest case it must handle |
|---|---|---|
| `ranked_bars` | count, share | horizontal bars in result order; `unmapped` is a drawn bar, never dropped; a weekly `gap:true` bar is a hatched no-data column |
| `line` | rate (count over week) | broken line + gap band + annotation over a `gap:true` week |
| `area` | count · week | filled band that drops out over a gap region rather than bridging it |
| `diverging_bars` | delta | zero-axis centered; sign encoded by color, magnitude by length; a weekly `gap:true` bar is a hatched no-data column |
| `histogram` | age, lifespan | fixed-edge bins, 0-day spike as its own bar; with one grouping → small multiples on shared scales |
| `stacked_bar` | share (two groupings) | cells sum to 1.0 globally; coverage pill on a thin classified slice; a weekly `gap:true` column renders as a hatched no-data column |
| `table` | count (any) | rows in result order as an HTML `<table>` (not SVG); the audit/trust anchor |

## Tasks

Each task is self-contained: it receives the Goal, its own paragraph, its acceptance criteria, and its invariants table — nothing else. Identifier and file names below are proposals for the implementer; behavior and invariants are the contract.

### Task 1 — chartTheme

Create a shared, static, plain-JS theme object (proposed `apps/web/lib/chart/chart-theme.ts`) that centralizes every geometric constant the layers need. It is imported statically by both the pure shaping layer and the React components, so it must be plain JS with no React and no browser API.

- Export a single `chartTheme` object; consumers accept a partial override via prop, deep-merged over the default. One vehicle keeps geometry consistent across encodings and overridable per chart.
- Carry margins, gutters, band padding, and per-encoding default `viewBox`/aspect ratio as implementer-chosen numeric constants — hardcoded JS numbers that echo the app's spacing rhythm in spirit, not read from any token source (the app's spacing tokens are CSS, unreadable as numbers during SSR — the reason Decision 5 exists). d3 geometry needs numbers, and only the reflow-breakpoint *values* are deferred (Open Questions); these v1 spacing constants are fixed here.
- Carry the genuinely uniform 7-day histogram edges `[7, 14, 21, 28, 35, 42, 49, 56, 63, 70]` (half-open `[lo, hi)`) with a `70+` open top bucket and the exactly-0 bucket `{0}` kept separate (Decision 2). One declared edge set gives every histogram server/client parity.
- Carry the reflow breakpoints as an ordered array of `ReflowBreakpoint` entries (`{ minWidth, tickDensity, rotateLabels, panelsPerRow }`, see Inter-layer contracts) that Task 4's `ResizeObserver` reads to pick tick density, label rotation, and small-multiple panels-per-row. The entry shape is fixed here; only the numeric thresholds are tuned during implementation (Open Questions), echoing the app's breakpoint rhythm so chart reflow stays aligned with the rest of the app.
- Keep the module free of React imports and browser globals so it is importable from an RSC and from the pure shaping layer. A client-only import here would poison the whole shaping layer's SSR-safety.

**Do not**

- Read spacing or breakpoints from CSS variables — they are not readable as numbers during SSR.
- Introduce React Context or a provider — a static import is the v1 vehicle; Context is a future runtime-theming option, not this.

**Acceptance criteria**

- [ ] A single statically-imported `chartTheme` object exports margins, gutters, band padding, per-encoding default viewBox/aspect, the histogram bin edges, and the reflow breakpoints as an ordered array of `ReflowBreakpoint` entries, and every value is a plain JS number or nested object/array of numbers.
- [ ] The histogram 7-day edges are exactly `7, 14, 21, 28, 35, 42, 49, 56, 63, 70` (half-open `[lo, hi)`, every bin 7 wide), yielding the full bucket set `{0}`, `[0,7)`, `[7,14)`, … `[63,70)`, `70+`, where `value === 0` matches `{0}` by precedence before the `[0,7)` bin and everything `>= 70` folds into `70+`. (Runnable — `pnpm typecheck` and a shaping `.test.ts`.)
- [ ] The module imports no React symbol and references no browser global, so `pnpm typecheck` passes and it is importable from a Server Component.
- [ ] A caller can override any theme value through a partial prop that deep-merges over the default without mutating the shared object.

| Invariant | Consequence if violated |
|---|---|
| Theme values are JS numbers, statically imported | SSR reads no geometry; charts render at zero size on first paint |
| Bin edges are fixed and declared here | Adaptive edges break SSR parity and invent boundaries |
| No React / no browser global in the module | The pure shaping layer that imports it loses SSR-safety |

### Task 2 — Shaping layer (pure, SSR-safe)

Create the dimensionless shaping module (proposed `apps/web/lib/chart/shaping.ts` and helpers). It turns a `MeasureResult` into plottable series: binned histograms, stacked shares, gap-split line/area segments, and variant-flagged bar data. It computes representation only, never a source-of-truth number. It is pure and SSR-safe. It reads `chartTheme` (Task 1) for bin edges. The shapes it emits are pinned in Inter-layer contracts.

```ts
// Proposed design — the shapes this layer emits (names may change, shapes are the contract).
type HistogramBin = { x0: number; x1: number | null; count: number; isZeroDay: boolean };
// x1 === null marks the `70+` open top; isZeroDay marks the separate exactly-0 `{0}` bucket.
type HistogramPanel = { group: string; bins: HistogramBin[] };
type HistogramPlot  = { panels: HistogramPanel[]; xDomain: number[]; yDomain: [number, number] };
// xDomain/yDomain are SHARED across every panel (emitted once) so small multiples compare honestly.

type BarVariant = "normal" | "unmapped" | "no-data";  // "no-data" = a weekly gap:true column
type ShapedBar  = { key: string; value: number; label: string; variant: BarVariant };
type StackedCell = { key: string; series: string; y0: number; y1: number; variant: BarVariant };

type PlotPoint = { x: Date; y: number };   // x in UTC domain units
type Segment   = PlotPoint[];              // contiguous, gap-free
type GapRegion = { start: Date; end: Date }; // in x-scale DOMAIN units, not pixels
type PlottedSeries = { segments: Segment[]; gapRegions: GapRegion[] };
```

- Import the `MeasureResult`/`MeasureRow` type from the engine module and the `Grouping`/`Measure` types and `Encoding` union from `@/lib/composition`; import no `postgres` symbol and no browser API. Purity is what lets this run in an RSC and under `pnpm test` without a DB.
- Bin `age`/`lifespan` raw fractional-day values with the fixed uniform 7-day `chartTheme` edges (half-open `[lo, hi)`): match `value === 0` to the separate `{0}` bucket by precedence, place every `value > 0` in the `[lo, hi)` bin containing it (`3.5` → `[0, 7)`), and fold everything `>= 70` into the `70+` open bucket (Decision 2). Emit `HistogramBin[]` (`isZeroDay` marks `{0}`, `x1: null` marks `70+`). Fixed uniform edges give SSR parity; the 0-day spike is a real finding kept visible.
- For a histogram with one grouping, produce one `HistogramPanel` per group and a **shared** x-domain and y-domain across panels (emit them once on the `HistogramPlot`), so small multiples compare honestly. Independent per-panel scales would make panels visually lie.
- Pre-split `line`/`area` rows into contiguous gap-free `Segment`s broken at every `gap: true` row, and emit the `gapRegions` separately (in x-scale domain units — UTC `Date`) so the composition layer can draw the no-data band and annotation (Decision 1). `.defined()` is at most an internal split detail here; the primitives never see gap-flagged data. Never emit a segment that spans a gap; a spanning segment is the smoothed lie the product exists to avoid.
- Emit bar-family data as `ShapedBar[]` (or `StackedCell[]` for `stacked_bar`) with a `variant` field: `unmapped` for the unmapped category, `no-data` for a weekly `gap:true` bar, `normal` otherwise (Decision 1). The flag carries both distinctions to the dumb `Bars` primitive; a `no-data` bar must never carry a real drawn value.
- Stack `share` rows for `stacked_bar` into `StackedCell` cumulative offsets without re-normalizing — `share` arrives summing to 1.0 globally, so the stack preserves that sum. Re-normalizing per panel would break the global-sum contract.
- Preserve result row order everywhere; shaping never sorts. `sort`/`limit` are the composition's, already applied by the engine.
- Fit no model (no LOESS/regression/spline) and interpolate across no absent point (Decision 6). v1 builds no drawn reading aid — its only consumer, scatter, is deferred; the quantile/median permission arrives with `scatter-with-trend`.

**Do not**

- Import `postgres`, or reference `window`, `document`, `ResizeObserver`, or any browser global — purity and SSR-safety are the whole point of this layer.
- Use `d3.bin()` or any adaptive binning — edges are the fixed `chartTheme` set.
- Fit a model, smooth, or interpolate across an absent point — forbidden by Decision 6, not merely deferred.
- Re-normalize `share` or re-sort any rows — both already arrive correct from the engine.

**Acceptance criteria**

- [ ] `grep -r postgres apps/web/lib/chart` is empty and the module references no browser global, so the shaping functions run under `pnpm test` (DB-free) and are importable from a Server Component.
- [ ] Binning `age`/`lifespan` values (fractional-day doubles) with the `chartTheme` edges matches `value === 0` to its own `{0}` bucket by precedence before the `[0, 7)` bin, places a `70`-or-greater value in the `70+` bucket, and places every other value in the half-open `[lo, hi)` bin containing it (e.g. `3.5` lands in `[0, 7)`, `7` lands in `[7, 14)`, `50` lands in `[49, 56)`).
- [ ] A single-grouping histogram produces one `HistogramPanel` per group whose x-domain and y-domain are shared across all panels.
- [ ] A `line`/`area` series containing a `gap: true` row is pre-split into gap-free segments that never span the gap, and the gap region is emitted separately in x-scale domain units for the band.
- [ ] Bar-family shaping emits a `variant` flag per datum: `unmapped` for the unmapped category, `no-data` for a weekly `gap:true` bar (carrying no drawn value), `normal` otherwise.
- [ ] Stacking a `share` result yields cumulative offsets whose top equals 1.0 (within floating tolerance) across the whole result, with no re-normalization applied.
- [ ] The shaping output preserves the input row order for every encoding.

| Invariant | Consequence if violated |
|---|---|
| No `postgres`, no browser global | Layer loses SSR-safety and DB-free testability |
| Fixed bin edges, 0-day bucket separate | Distribution shape stops being a reviewable decision |
| Segments never span a `gap: true` row | The chart draws a value no run backs (the core lie) |
| No model fitting, no interpolation across absent points | v1 draws a curve no data backs; the parity guarantee erodes |

### Task 3 — Primitives

Create the app-owned mark components (proposed `apps/web/components/chart/primitives/`): `Bars`, `Line`, `Area`, `Axis`, `StackedBars`, plus `GapBand` (the shaded no-data band + annotation) and `CoveragePill`. Each emits plain SVG JSX carrying Tailwind classes and receives its scales as props. They are the leaf layer; they compute no scale and hold no responsive logic.

```ts
// Proposed design — the shapes each primitive consumes (names may change, shapes are the contract).
type BarVariant  = "normal" | "unmapped" | "no-data";
type ShapedBar   = { key: string; value: number; label: string; variant: BarVariant };  // Bars
type StackedCell = { key: string; series: string; y0: number; y1: number; variant: BarVariant }; // StackedBars
type PlotPoint   = { x: Date; y: number };
type Segment     = PlotPoint[];                 // Line / Area take Segment[] (one path per segment)
type GapRegion   = { start: Date; end: Date };  // GapBand takes GapRegion (domain units) + xScale
type Coverage    = { classified: number; total: number };  // CoveragePill takes a value, not a scale
```

- The scale-consuming primitives (`Bars`, `Line`, `Area`, `Axis`, `StackedBars`, `GapBand`) accept scales (`xScale`, `yScale`, color scale where relevant) as props and use them read-only; a primitive that builds its own scale breaks the "d3 as math, composition owns scales" split and desyncs from sibling marks.
- Emit only `<rect>`, `<path>`, `<text>`, `<line>`, `<g>` etc. with Tailwind classes for color/stroke; never call any imperative d3 DOM API (`selection.append`, `axisBottom`). App-owned JSX is what carries the design system and stays reviewable.
- `Line` renders **one path per pre-split segment** (Task 2), so a gap is a break between drawn paths, never a bridge (Decision 1); `Area` fills per segment so the band drops out over a gap. The primitive never applies `.defined()` to gap-flagged data — shaping already split the series.
- `Bars` renders each `ShapedBar` by its `variant`: a `normal` bar normally, an `unmapped` bar visually distinct, and a `no-data` bar (a weekly gap) as a hatched placeholder column at the axis baseline (Decision 1). The variant decides the rendering; `Bars` holds no gap logic of its own.
- `StackedBars` renders a `StackedCell[]` (`{ key, series, y0, y1, variant }`) as stacked rects from the cumulative `y0`/`y1` offsets, taking its scales as props, and renders a `no-data`-variant cell (a weekly `gap:true` week) as a hatched no-data segment — mirroring how `Bars` draws its no-data column — never a real stacked value. It is the only primitive that draws a `StackedCell`, keeping `stacked_bar` (Task 5) a composition of this mark rather than a hand-rolled stack.
- `Bars` renders no value labels; the encoding component (Tasks 5–6) composes the value-label `<text>` around the bars, keeping `Bars` minimal. One layer owns labels so the primitive stays a pure mark.
- `GapBand` takes `xScale` as a prop and receives its `GapRegion` (`{ start, end }`) in x-scale domain units (UTC `Date` for time series), mapping domain→pixels through that scale; it draws the shaded gray region and its annotation ("gray marks a gap in collected data"). It is scale-consuming because the band spans a domain range, and it is the visible half of the honesty contract for the line/area family.
- `CoveragePill` renders from a passed `Coverage` value (not a scale); it draws the classified/total pill and nothing of the cross-surface coverage chrome.
- Compose Tailwind classes through the project's `cn()` helper and keep custom class names outside the `text-`/`bg-`/`border-`/`fill-` namespaces where a bare word would be dropped by tailwind-merge (web-guide §Color namespaces). A silently dropped color class ships a chart with the wrong or no color.
- Primitives are presentational and hold no `ResizeObserver` — responsiveness lives one layer up (Task 4). Keeping marks dumb lets them render identically under SSR and after reflow.

**Do not**

- Build or derive a scale inside a primitive — scales arrive as props.
- Call any imperative d3 selection/axis API — emit JSX only.
- Use a custom Tailwind utility in the `text-`/`bg-`/`border-`/`fill-` namespaces (dropped by `cn()`), or any deprecated utility listed in web-guide (`rounded`, `shadow`, …).

**Acceptance criteria**

> Verification: `vitest` runs node-only here (no component-test environment), so the runnable gates are `pnpm typecheck`, `pnpm build-storybook` (compile-only), and `grep`; the visual/rendering ACs are verified by Storybook review. Each AC is tagged with how it is checked.

- [ ] (Runnable — `pnpm typecheck`, `pnpm build-storybook`) `Bars`, `Line`, `Area`, `Axis`, `StackedBars`, and `GapBand` each render from scales passed as props (`GapBand` takes `xScale` and maps its domain-unit `GapRegion` to pixels through it); `CoveragePill` renders from a passed `Coverage` value and consumes no scale; all emit only SVG JSX with Tailwind classes and call no imperative d3 DOM API.
- [ ] (Story review) `Line` given pre-split segments renders one path per segment and draws none across a gap; `Area` fills per segment so the band drops out over the gap.
- [ ] (Story review) `Bars` renders a `no-data`-variant datum as a hatched placeholder column at the axis baseline and an `unmapped`-variant datum as a visually distinct bar, driven only by the `variant` field.
- [ ] (Story review) `StackedBars` renders a `StackedCell[]` as stacked rects from the cumulative `y0`/`y1` offsets, renders a `no-data`-variant cell as a hatched no-data segment rather than a real stacked value, takes its scales as props, and emits only SVG JSX with Tailwind classes, matching the other marks' rules.
- [ ] (Story review) `GapBand` renders a shaded region plus the no-data annotation over a passed domain-unit `GapRegion`, mapped to pixels through its `xScale`.
- [ ] (Runnable — `grep`) Every custom Tailwind utility used sits outside the `text-`/`bg-`/`border-`/`fill-` namespaces, so `cn()` does not drop it.
- [ ] (Runnable — `pnpm typecheck`, `pnpm build-storybook`) Both commands pass with the primitives in place.

| Invariant | Consequence if violated |
|---|---|
| Scales arrive as props | Marks desync from siblings; the layer split collapses |
| SVG JSX only, no d3 DOM | Marks lose Tailwind classes and reviewability |
| Custom classes avoid color namespaces | `cn()` silently drops the class; chart renders wrong |

### Task 4 — Composition layer: scales, viewBox, responsive wrapper, coverage boundary

Create the composition infrastructure (proposed `apps/web/components/chart/composition-chart.tsx` and scale/axis helpers) that the seven encoding components share. It owns scales, margins, axes, and viewBox math; the measured responsive wrapper; and the coverage-prop discriminated union with its runtime backstop. This is the layer that makes charts client components. It consumes Task 2 shaping output and places Task 3 primitives; the encodings (Tasks 5–6) build on it.

```ts
// Proposed design — the coverage prop contract (names may change, shapes are the contract).
type Coverage = { classified: number; total: number };
type ClassifiedResult = MeasureResult & { denominator: Coverage };  // denominator present
type FullCorpusResult = MeasureResult & { denominator?: never };    // denominator absent
type CoverageProps =
  | { result: FullCorpusResult }
  | { result: ClassifiedResult; coverage: Coverage };
```

- Construct **all** scales here with `d3-scale` and pass finished scales as props to the encoding components (Tasks 5–6), which build none. Centralizing scale construction is what keeps encodings and primitives dumb and keeps sibling marks in sync.
- Produce the scale kinds each encoding needs: band + linear for `ranked_bars` and `stacked_bar`; a zero-centered **symmetric** linear domain for `diverging_bars`; UTC `d3-time` + linear for `line`/`area`; band-over-declared-edges + a **shared** linear y-scale for `histogram` and its small multiples. (`table` needs no scale — see Task 5.) Enumerating the kinds here is what lets the encodings stay scale-free.
- Use `d3-time` **UTC** scales and `d3-time-format` for any time axis, so server and client compute identical ticks and there is no hydration mismatch. A local-time scale would tick differently across timezones and desync SSR from hydration.
- Own margins/gutters/viewBox from `chartTheme` (Task 1). Centralizing this geometry keeps every encoding consistent and overridable in one place.
- Implement responsiveness as progressive enhancement (Decision 4): server-render a fixed-`viewBox` fallback (correct aspect, coverage pill and gap band present on first paint) from `chartTheme`, then a client `ResizeObserver` on a measured wrapper reflows tick density, label rotation, and panels-per-row to the real width. The deterministic fallback plus UTC scales guarantee the SSR HTML matches the first client render.
- These components are `"use client"` because `ResizeObserver` is browser-only; state that a client component still SSRs its initial HTML, so first paint is server-rendered and there is no blank flash. Losing SSR first paint would regress perceived load.
- Type the props as a discriminated union **over result type**, not an optional `coverage?` (which cannot error on absence): `{ result: FullCorpusResult } | { result: ClassifiedResult; coverage: Coverage }`, where `ClassifiedResult`/`FullCorpusResult` narrow `MeasureResult` on `denominator` presence (see Inter-layer contracts). A value statically typed as `ClassifiedResult` without `coverage` is then a compile error (Decision 3). The type is the first line of defense against a silent naked slice.
- Add the runtime backstop: when an untyped/cast result (`any`, or parsed JSON at the model-emitted boundary) carries a `denominator` (classified) but no `coverage` prop, refuse to render the slice and render a refusal state instead of a naked chart. A value statically typed `MeasureResult`/`ClassifiedResult` is already rejected by the props union at compile time, so the backstop fires only on dynamically-typed data.
- Render the coverage pill from the passed `coverage` value via the `CoveragePill` primitive; own the pill and the prop enforcement, and nothing of the shared coverage chrome across chat and page (that is `composition-frame-and-recovery`). Drawing the pill from a passed value keeps this layer's coverage responsibility crisp.

**Do not**

- Use a non-UTC time scale — server/client tick drift causes hydration mismatch.
- Let a chart render a classified result (`denominator` present) without a `coverage` value — the runtime backstop must refuse.
- Absorb the shared coverage/recovery/empty-state chrome — that is a later spec; render the pill and stop.
- Build a scale inside a primitive or duplicate scale construction per encoding — scales are built once, here.

**Acceptance criteria**

> Verification: `vitest` runs node-only here (no component-test environment), so the runnable gates are `pnpm typecheck` (incl. the compile-error type test), `pnpm build-storybook`, and `grep`; the visual/rendering ACs are verified by Storybook review. Each AC is tagged with how it is checked.

- [ ] (Story review — hydration) All time axes use UTC `d3-time` scales, so the server-rendered ticks and the client-hydrated ticks are identical (no hydration-mismatch warning).
- [ ] (Story review) A chart server-renders a fixed-`viewBox` fallback with the correct aspect, coverage pill, and gap band present before hydration, then reflows tick density, label rotation, and panels-per-row on `ResizeObserver` resize.
- [ ] (Runnable — `pnpm typecheck`) Passing a value statically typed as `ClassifiedResult` without `coverage` is a TypeScript compile error, verified by an `@ts-expect-error` type-level test.
- [ ] (Story review) A chart handed an untyped/cast value (`any`, or parsed JSON at the model-emitted boundary) whose `denominator` is present but with no `coverage` prop renders a refusal state and draws no slice — the runtime backstop fires only on dynamically-typed data, since a statically typed `MeasureResult`/`ClassifiedResult` without `coverage` is already rejected by the union.
- [ ] (Story review) A full-corpus result (no `denominator`) renders with no coverage pill and no refusal.
- [ ] (Story review + `grep`) The coverage pill renders from the passed `coverage` value and this layer contains none of the shared cross-surface coverage chrome.

| Invariant | Consequence if violated |
|---|---|
| UTC time scales | Hydration mismatch; server and client disagree on ticks |
| SSR fixed-viewBox fallback + client reflow | Blank flash or non-responsive charts |
| Classified-without-coverage: compile error + runtime refusal | A naked classified slice ships without its denominator |

### Task 5 — Bar-family encodings: ranked_bars, diverging_bars, stacked_bar, table

Build the categorical/ranked encoding components (proposed `apps/web/components/chart/encodings/`, one family per file) on Task 4's infrastructure, Task 3's primitives, and Task 2's shaping. Separate file from Task 6 so the two encoding sets land concurrently without shared edits.

```ts
// Proposed design — the shapes this task feeds to Task 3 primitives (shapes are the contract).
type BarVariant  = "normal" | "unmapped" | "no-data";  // "no-data" = a weekly gap:true column
type ShapedBar   = { key: string; value: number; label: string; variant: BarVariant };  // → Bars
type StackedCell = { key: string; series: string; y0: number; y1: number; variant: BarVariant }; // → StackedBars
```

- `ranked_bars`: horizontal bars in result order (never re-sorted); the encoding composes value labels at bar ends (`Bars` renders no labels — Task 3); render an `unmapped` category as a visible, visually-distinct bar, never a dropped row. A dropped `unmapped` bar would claim coverage the market grouping lacks.
- `diverging_bars` (`delta`): a zero-centered axis with a symmetric domain around 0, color encoding sign and length encoding magnitude. Sign is the story; a non-symmetric domain would distort the comparison of gains vs. losses.
- `stacked_bar` (`share`, two groupings): renders Task 2's `StackedCell[]` through the `StackedBars` primitive (Task 3), whose cumulative `y0`/`y1` offsets sum to 1.0 across the whole result; the encoding builds no marks of its own. Global (not per-panel) summing is the grammar's contract for two-grouping share.
- The coverage pill is owned and rendered by Task 4's `CompositionChart` wrapper for any classified result — `stacked_bar` and equally `ranked_bars` over a classified grouping — so no Task 5 encoding re-implements `CoveragePill` or decides pill visibility. Forking pill logic here would split the coverage contract Task 4 owns.
- For any weekly bar series, render a `gap:true` week as a hatched no-data column driven by the `no-data` variant (Decision 1) — drawn by `Bars` for `ranked_bars`/`diverging_bars` and by `StackedBars` for `stacked_bar`, never a real 0 bar and never a dropped row. A phantom 0 bar asserts a value no run backs.
- `table`: rows rendered in result order as an HTML `<table>` (**not** SVG), tabular-numeric alignment on the value column. It is **exempt** from Task 4 scales and Task 3 primitives — an HTML table needs neither. Every narrated claim links back to this; row order must equal result order.
- For the SVG bar encodings (`ranked_bars`, `diverging_bars`, `stacked_bar`), take scales from Task 4 and marks from Task 3; add no scale-building or responsive logic here. Encodings are compositions of the lower layers, not new infrastructure.

**Do not**

- Re-sort or re-order rows — render result order.
- Drop an `unmapped` (or any) category row, or draw a weekly gap as a real 0 bar — draw the category, draw the gap as a no-data column.
- Re-normalize `share` — the stack preserves the engine's global 1.0.
- Render `table` as SVG or route it through Task 4 scales / Task 3 primitives — it is an HTML `<table>`.
- Build scales or add `ResizeObserver` here — those belong to Task 4.

**Acceptance criteria**

> Verification: `vitest` runs node-only here (no component-test environment), so the runnable gates are `pnpm typecheck`, `pnpm build-storybook`, and `grep`; the visual/rendering ACs are verified by Storybook review. Each AC is tagged with how it is checked.

- [ ] (Story review) `ranked_bars` renders bars in exact result order and draws an `unmapped` category as a distinct visible bar.
- [ ] (Story review) `diverging_bars` renders a zero-centered axis with a symmetric domain, sign encoded by color and magnitude by length.
- [ ] (Story review) `stacked_bar` composes the `StackedBars` primitive to render cells summing to 1.0 across the whole result (not per panel) and builds no stacked rect of its own.
- [ ] (Story review + `grep`) The coverage pill for a classified result comes from Task 4's `CompositionChart` wrapper — for any classified encoding (`stacked_bar`, or `ranked_bars` over a classified grouping) — and no Task 5 encoding re-implements `CoveragePill`.
- [ ] (Story review) A weekly bar series with a `gap:true` week renders that week as a hatched no-data column (via `Bars`, or `StackedBars` for `stacked_bar`), not a 0 bar and not a dropped row.
- [ ] (Story review) `table` renders rows in result order as an HTML `<table>` with the value column right-aligned as tabular numerals, using no Task 4 scale and no Task 3 primitive.
- [ ] (Runnable — `grep` + `pnpm typecheck`) Each SVG bar encoding (`ranked_bars`, `diverging_bars`, `stacked_bar`) consumes Task 4 scales and Task 3 primitives and contains no scale construction or `ResizeObserver` of its own; `table` is exempt.

| Invariant | Consequence if violated |
|---|---|
| Result order preserved | The chart reorders a question the engine already answered |
| `unmapped` is drawn | Chart implies coverage the grouping does not have |
| A weekly gap bar is a no-data column, never a 0 bar | The chart asserts a value no run backs |
| Two-grouping share sums globally to 1.0 | Per-panel normalization misstates the distribution |
| `table` is HTML, exempt from scales/primitives | Forcing SVG on a table breaks the audit anchor's alignment |

### Task 6 — Temporal & distribution encodings: line, area, histogram (+ small multiples)

Build the temporal and distribution encoding components (proposed `apps/web/components/chart/encodings/`, separate file(s) from Task 5) on Task 4's infrastructure, Task 3's primitives, and Task 2's shaping. Separate file from Task 5 for concurrent landing.

```ts
// Proposed design — the shapes this task consumes from Task 2 (shapes are the contract).
type HistogramBin = { x0: number; x1: number | null; count: number; isZeroDay: boolean };
// isZeroDay marks the separate exactly-0 `{0}` bucket; x1 === null marks the `70+` open top.
type PlotPoint = { x: Date; y: number };
type Segment   = PlotPoint[];               // Line/Area render one path per gap-free segment
type GapRegion = { start: Date; end: Date }; // domain units → GapBand maps via xScale
```

- `line` (`rate`): render the pre-split segments (Task 2) as a broken line via `Line`, and draw the `GapBand` plus annotation over every gap region. Never interpolate, tween, or drop to 0 across a gap (Decision 1). The break is the finding.
- `area` (`count` · week): render per-segment fills so the band drops out over a gap region rather than bridging it, matching the line's honesty rule. A bridged fill asserts collected data that does not exist.
- `histogram` (`age`/`lifespan`): render the fixed uniform 7-day bins from Task 2 with the exactly-0 bucket visually distinct, and label the declared edges (`0`, `7`, `14`, … `70`, `70+`). Deterministic uniform edges are what make the distribution reviewable and SSR-stable.
- Small multiples: when the histogram has one grouping, render one panel per group on the **shared** scales from Task 2, with panels-per-row reflowing via Task 4's wrapper. Shared scales are what let panels be compared; independent scales would lie.
- Take scales from Task 4 and marks from Task 3; add no scale-building here. Same layer discipline as Task 5.

**Do not**

- Interpolate, smooth, tween, or zero-fill across a `gap: true` region — draw the break and the band.
- Use adaptive binning or per-panel histogram scales — fixed edges, shared scales.
- Build scales or duplicate responsive logic — those live in Task 4.

**Acceptance criteria**

> Verification: `vitest` runs node-only here (no component-test environment), so the runnable gates are `pnpm typecheck`, `pnpm build-storybook`, and `grep`; the visual/rendering ACs are verified by Storybook review. Each AC is tagged with how it is checked.

- [ ] (Story review) `line` renders one path per pre-split segment over a `gap: true` week with a gray no-data band and its annotation, and draws no path across the gap.
- [ ] (Story review) `area` renders a fill that drops out over a gap region rather than bridging it, and draws the `GapBand` plus the no-data annotation over that region, mirroring `line`.
- [ ] (Story review) `histogram` renders the fixed uniform 7-day `chartTheme` edges with the exactly-0 bucket visually distinct and the declared edges labeled (`0`, `7`, … `70`, `70+`).
- [ ] (Story review) A single-grouping histogram renders one panel per group on shared x/y scales, with panels-per-row reflowing to width.
- [ ] (Runnable — `grep` + `pnpm typecheck`) Each encoding consumes Task 4 scales and Task 3 primitives and builds no scale of its own.

| Invariant | Consequence if violated |
|---|---|
| No draw across a gap | The signature honesty finding is erased |
| Fixed edges, shared small-multiple scales | Distribution shape and cross-panel comparison stop being trustworthy |

### Task 7 — Storybook gallery (the proof)

Add a Storybook-driven gallery (proposed `apps/web/components/chart/chart-gallery.stories.tsx`, under `components/**` so the glob loads it) that renders all seven encodings from `MeasureResult`-shaped **fixtures** — not a live DB — mirroring how `composition-grammar` shipped its Inspector story. This is the proof that the foundation is exercised, not an unrendered stub. It uses inline/representative fixtures; the deterministic shared corpus is a separate later item (`demo-fixture-corpus`).

- Render one story per encoding from inline `MeasureResult` fixtures, so the gallery runs with no DB and no engine call. Fixtures keep the proof pure and reviewable.
- Include the degenerate/honest cases explicitly: a `line` and an `area` each with a failed-run gap week (broken line + band + annotation); a weekly bar series with a `gap:true` week rendered as a hatched no-data column; a `histogram` with the 0-day spike; a `lifespan` small-multiples story with shared scales; a `stacked_bar` with a coverage pill on a thin classified slice; and a full-corpus bar chart with a distinct `unmapped` bar. These are the cases the design exists to get right.
- Add a story rendering the runtime refusal state (the Task 4 backstop — a dynamically-typed classified result with no `coverage`) and a separate `@ts-expect-error` type-level test for the classified-without-coverage compile error. Two artifacts: one story-reviewed, one typechecked — the honesty guarantee is visible in the gallery and enforced at compile time.
- State inline that fixtures are throwaway/representative and the shared deterministic corpus is `demo-fixture-corpus`. This keeps the scope boundary explicit for the next reader.
- Load the font decorator convention (web-guide §Storybook) so text renders in the app font, and keep the story file the only place fixtures live. Stories elsewhere silently never load.

**Do not**

- Read from Postgres or call `runComposition` — the gallery runs on fixtures only.
- Place the story outside `components/**` — Storybook's glob will not find it.
- Build the shared deterministic corpus here — that is `demo-fixture-corpus`.

**Acceptance criteria**

- [ ] The gallery renders all seven encodings from inline `MeasureResult` fixtures with no DB access, and `pnpm build-storybook` passes.
- [ ] Stories exist for each honest case: `line` gap, `area` gap, a weekly bar gap rendered as a no-data column, histogram 0-day spike, `lifespan` small multiples with shared scales, `stacked_bar` with a coverage pill on a thin classified slice, and a full-corpus bar with a distinct `unmapped` bar.
- [ ] (Story review) A Storybook story renders the runtime refusal state — a dynamically-typed classified result with no `coverage` shown refusing rather than drawing a naked slice.
- [ ] (Runnable — `pnpm typecheck`) An `@ts-expect-error` type-level test asserts that a statically-typed `ClassifiedResult` without `coverage` is a compile error.
- [ ] The story file documents that its fixtures are representative/throwaway and names `demo-fixture-corpus` as the source of the shared deterministic corpus.

| Invariant | Consequence if violated |
|---|---|
| Fixtures only, no DB | The proof couples to the engine and stops being a pure foundation check |
| Every honest case has a story | A degenerate case ships unexercised and regresses silently |
| Story lives under `components/**` | Storybook silently never loads it |

## Not doing

- The `scatter` encoding and its quantile band — `scatter-with-trend`. The reading-aid *guardrail* (Decision 6) is stated here because it governs all shaping, but v1 builds no drawn reading aid; the quantile/median permission arrives with that spec, alongside its first consumer.
- Radial/pie `share` — `radial-share-encoding`. Render only the seven grammar encodings.
- A `survival` measure — `survival-measure`; and month-grain time aggregation — `time-grain-modifier`. This layer renders weekly-grain data as the grammar produces it.
- The `/view` route, the tool binding, and chat transport — other specs.
- The shared coverage chrome / recovery / empty-state treatment across chat and page — `composition-frame-and-recovery`. This spec renders the pill from a passed value and stops.
- Any model-fitting, smoothing, or interpolation — forbidden by Decision 6, not deferred.
- Live DB wiring — shaping is pure; stories run on fixtures.
- The shared deterministic demo fixture corpus — `demo-fixture-corpus`. This spec uses inline/representative fixtures.
- Runtime theme-switching via React Context — a future option once runtime theming is wanted, not v1 (Decision 5).

## Build order

**Phase 1 (sequential):** Task 1 — `chartTheme`; every later layer reads its geometry and edges.
**Phase 2 (sequential):** Task 2 — pure shaping layer; consumes `chartTheme`, defines the plottable series the components render.
**Phase 3 (sequential):** Task 3 — primitives; the leaf marks the composition layer places. (Per the layering rule, shaping + theme land before primitives.)
**Phase 4 (sequential):** Task 4 — composition infrastructure (scales, viewBox, responsive wrapper, coverage union + backstop); the encodings build on it.
**Phase 5 (concurrent):** Task 5 (bar-family encodings) and Task 6 (temporal & distribution encodings) — separate files, both consume Phase 2 shaping, Phase 3 primitives, and Phase 4 infrastructure.
**Phase 6 (sequential):** Task 7 — the Storybook gallery, once all seven encodings exist to render.

## Done when

The automated honesty gates are the pure shaping layer (Task 2, `pnpm test`), the classified-without-coverage compile-error type test (Task 4, `pnpm typecheck`), and `grep` purity checks. `vitest` runs node-only with no component-test environment, so every visual AC is verified by Storybook review, not a runnable assertion. Each bullet below is tagged with how it is checked.

- (Runnable — `pnpm typecheck`) `pnpm typecheck` passes; a statically-typed classified composition without `coverage` is a compile error you can see (the `@ts-expect-error` type test).
- (Runnable — `pnpm test` + `grep`) `pnpm test` (DB-free) passes for the shaping layer, and `grep -r postgres apps/web/lib/chart` is empty — the shaping layer is pure.
- (Runnable — `pnpm build-storybook`) `pnpm build-storybook` passes and the gallery renders all seven encodings from fixtures.
- (Story review) A `line`/`area` fixture with a `gap: true` week renders a broken line/dropped band plus the no-data annotation, never a bridge or a zero.
- (Story review) A `histogram` fixture renders the fixed `chartTheme` edges with the 0-day bucket as its own bar; a single-grouping histogram renders small multiples on shared scales.
- (Story review) A classified `stacked_bar` fixture renders a coverage pill (from the Task 4 wrapper); a dynamically-typed classified result handed no `coverage` is refused rather than drawn naked; a full-corpus bar renders a distinct `unmapped` bar.
- (Story review) Charts SSR a fixed-viewBox first paint and reflow to width on the client with no hydration-mismatch warning.

## Open questions

- **Reflow-breakpoint numeric thresholds.** The `ReflowBreakpoint` entry shape is pinned (Inter-layer contracts); only its numbers — the `minWidth` cutoffs, per-width `tickDensity`, and the panels-per-row counts at which a measured wrapper beats a fixed viewBox before panels get unreadable — are `chartTheme` values to tune during implementation, not fixed here.
