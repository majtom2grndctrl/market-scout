# Composition Grammar

> **Purpose:** The reframe of record for the Compositional Analysis Surface. States the product model once.
> **Sources:** Builds on `ui-view-catalog.md` (corpus, coverage, out-of-scope) and `agent-ui-landscape.md` (transport, charts).
> **Status:** Reframe of record. Supersedes the fixed-catalog product model in `ui-view-catalog.md`. Source doc for the `composition-grammar` spec.

---

## The Turn

The catalog model was seven hand-built views — one tool, one chart, one page each. The model's job was to pick the right one. That tests selection.

The grammar model exposes primitives and lets the agent combine them. The model composes; prompting steers what it reaches for. That tests composition.

The project exists to demo designing for AI interaction, not to ship a market-intel product. Composition is the version that matches the thesis. The catalog is a product; the grammar is a research instrument.

## The Grammar

An analysis is one point in a small space. One measure, resolved over a grouping and filter, rendered by an encoding — plus modifiers:

- **Measure** (one per composition) — count, delta, age, lifespan, rate, share. Backend-computed.
- **Grouping** — company, role, specialization, skill, seniority, function, weekly bucket.
- **Filter** — scope to a company, role, skill, seniority, or window.
- **Encoding** — line, area, diverging bars, ranked bars, histogram, stacked bar, table. Picked by result shape.
- **Modifiers** — cohort (open/closed/all), sort, limit. Ordering is part of the question, so it serializes into the link.

Movers is `measure:delta × group:company × sort:desc × limit:20 × encode:diverging_bars`. Role Lifecycle is `measure:lifespan × cohort:closed × encode:histogram`. The catalog views are points; the space holds combinations no one catalogued — lifespan by skill for senior roles only.

**Absence is the crown jewel.** Fetch-run-aware open/closed is the one data type no scraped board has. Not one view — a cohort choice on the count measures (currently-open, closed, or all), and the very definition of the time measures (`age` is open, `lifespan` is closed). Center the experiment here.

## Two Invariants

**Backend computes every primitive.** The model selects and combines, never calculates. It narrates aggregates a tool returned. Fabricated numbers stay structurally impossible. This is the line between the grammar and text-to-SQL, which `ui-view-catalog.md` rejected — hand the model SQL and the invariant is gone.

**Unreliable types are absent from the vocabulary.** No compensation measure. No geography grouping. Out-of-scope is structural, not a per-question refusal — an unanswerable question is uncomposable, and the "no" comes for free.

## Serialization And Pages

A composition serializes to its deep link and back. The link is the call, so chat and page cannot disagree — same query at two altitudes.

No hand-built `/movers` or `/trends`. One composition renderer, at chat scale and at page scale. The Posting Explorer stays the records terminus: every composition drills to the rows behind it.

## Coverage Rides Along

A grouping over a classified dimension returns its denominator inline. A chart over the 35% classified slice cannot imply full coverage. The tool returns it; the chat narrates it aloud. Not a footnote.

## The Quality Rubric

The agent will compose combinations that are novel and meaningless. The experiment needs a readout, so "interesting" is scored on three axes:

- **Novel** — not a preset. A combination the catalog never held.
- **Sound** — measure and grouping fit; the crossing carries signal, not noise.
- **Coverage-honest** — discloses its denominator; never over-reads a thin slice.

Prompting is the knob. `composition-quality-evals` scores prompt variants against this rubric per model, so a rewrite is measured, not argued.

## Seed Compositions

Five catalog views reduce to a single-measure composition and survive as saved compositions: Movers, Demand Trend, Role Lifecycle (full corpus), Demand by Function and Seniority Mix (behind the denominator). Each seeds a cold-start chip — a live-data answer the user did not type — and proves the grammar spans it. Company Profile is a *composite* — several compositions in one layout, a later frame concern — and Skill Overlap needs a co-occurrence primitive the vocabulary doesn't yet have; both wait. Open composition on top; known-good bookmarks underneath.

## What Still Holds

- `ui-view-catalog.md` — the corpus figures, coverage split, shared params, and out-of-scope list stay valid. Its product model — seven fixed views as the deliverable — does not.
- `agent-ui-landscape.md` — build the contract, don't buy CopilotKit's; d3 as math, marks by the app. Both stand, and the grammar strengthens the first: the vocabulary is now the whole designed artifact.
