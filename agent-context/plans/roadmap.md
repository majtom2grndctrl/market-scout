# Plans Roadmap

> **Read this when:** choosing the next plan to draft, review, promote, or build.
> **Key invariant:** checkboxes are roadmap rollup only.
> **Related:** `agent-context/lib/style-guide.md` §Documentation Lifecycle, `roadmap-archive/`

---

## How To Read This

Spec folder names are stable IDs.

Checkbox meaning:

| Marker | Meaning |
|---|---|
| `[ ]` | Active roadmap item. Still relevant to planning. |
| `[x]` | Finished. Safe to look past unless doing history. |

If checkbox and folder location disagree, folder location wins. Fix the roadmap when noticed.

Spec folder names must be unique across plan lifecycle folders.

Move completed epics out of this file at the end of the quarter. Archive them in `roadmap-archive/YYYY-QN.md`.

## Epic: Browser-Led Market Scouting

Goal: Agent can discover companies from web sources, investigate them in a browser, and safely onboard supported ATS boards.

### Milestone: Discovery Run Foundation

- [x] `browser-led-discovery-runs`
  Define the browser-led discovery run. It should cover source inputs, candidate records, provenance, statuses, dedup preflight shape, and run summaries. It is the foundation for multi-company pages and recent-news scouting.

- [x] `discover-and-onboard-agent-loop`
  Done. Browser-observed URL evidence now flows through shared ATS detection, the non-mutating `detect_ats` MCP preflight, and `add_company` as the verification and write gate.

### Milestone: Safe Onboarding

- [ ] `stage-company-seed-patch`
  Reconcile companies added through MCP with the canonical seed file. It should produce a human-reviewed patch, not mutate source files as a hidden side effect of `add_company`.

### Milestone: Source Recipes

- [ ] `discovery-source-recipes`
  Add browser recipes for common sourcing modes. Cover one page that mentions many companies, and recent articles from a tech-news source. Recipes should feed the discovery run workflow rather than define a separate pipeline.

## Epic: Compositional Analysis Surface

Goal: Web agent composes analyses from a small vocabulary — measure × grouping × filter × encoding — and never emits a number itself; the backend computes every aggregate. The experiment: how prompting steers which combinations it reaches for, and whether they land novel, sound, and coverage-honest. Reframes the fixed catalog in `../../research/ui-view-catalog.md` — the seven views survive as seed compositions, not hand-built screens. Transport rationale: `../../research/agent-ui-landscape.md`.

### Milestone: Grammar

- [x] `composition-grammar`
  Define the vocabulary and its invariant. One measure — count, delta, age, lifespan, rate, share — over a grouping (company, role, specialization, skill, seniority, function, weekly bucket) and filter, rendered by an encoding, with cohort, sort, and limit modifiers. Cohort carries the fetch-run-aware open/closed/all split. Backend computes every primitive; the model selects and combines, never calculates. Unreliable data types are absent from the vocabulary, not refused per question — no compensation measure, no geography grouping — so out-of-scope is structural. A composition serializes to the deep link and back. Versioned as a public contract.

- [ ] `measure-engine`
  Compute a serialized composition into typed rows plus a coverage denominator. Lives in `apps/web/lib/db`, over the read-model views. Owns delta, lifespan, rate, and share on top of the open/closed definition the read model already fixes. Classified groupings return their denominator inline, so a chart over the 35% classified slice cannot imply full coverage. Never hands raw SQL to the model.

- [ ] `chart-primitives`
  Define the chart layer. `d3-scale`, `d3-shape`, `d3-array` supply scales and path geometry; the app owns every mark, so charts inherit design tokens rather than a library theme. Three layers — dimensionless shaping, a composition layer owning scales and margins, and primitives that receive scales as props and never build their own. Render target is any shape the grammar produces, not a fixed set of forms. Settle the fixed-`viewBox` default and what earns a measured wrapper, UTC scales so server and client render identical ticks, and a failed-run window as a gap, never an interpolated line. Absence is data; a chart that smooths over it lies.

- [ ] `cooccurrence-measure`
  Add a co-occurrence measure. Pairwise overlap becomes composable: which skills appear together on a posting, which skills attach to a role. The grammar shapes one measure over one grouping; two terms from the same dimension are a relationship that shape cannot hold, so co-occurrence was deferred. Define the primitive, its symmetric-pair rows, and a denominator honest about the classified slice it reads. `measure-engine` already spans the plain case — a grouping filtered by another dimension, such as roles filtered by a skill — so this spec owns only the pairwise measure the current vocabulary cannot express. Unlocks the Skill Overlap seed composition. Composite (Company Profile) is the sibling deferred primitive; it stays a separate spec.

#### Candidates — deferred from chart-primitives design

Surfaced while designing `chart-primitives`. Each is a vocabulary or encoding extension that waits on its own spec; none blocks the demo chain.

- [ ] `scatter-with-trend`
  Add a `scatter` encoding for per-posting measures — raw points plus an optional quantile band (p25/median/p75) computed in the shaping layer as a reading aid, never a fitted model. Honest because every point is a real posting and the band only summarizes points shown. Gives `age`/`lifespan` a distribution form beyond the histogram. Needs an encoding-enum addition in the grammar; the band math stays chart-side because it is drawn, not narrated.

- [ ] `survival-measure`
  Add a survival measure — postings still open at day *t*, a real aggregate over fetch-run-aware absence, engine-computed as source of truth. In this market it plateaus on the persistent core rather than decaying to zero; the plateau is the finding. Distinct from `lifespan`: not derivable from the per-posting values, so it is a new measure, not a chart restyle.

- [ ] `time-grain-modifier`
  Add a `grain` modifier (`week | month`) on time groupings, engine-aggregated from the weekly source. Daily stays absent for cause — too few fetch days to render honestly. Gated on data volume: monthly over the current ~10-week window yields two or three points, so this earns its spec once the corpus spans a quarter or more.

- [ ] `radial-share-encoding`
  Add a hero radial encoding for `share` — a nested-donut form sizing each circle by its base-N as a confidence cue, so thin classified slices read as visually quiet. A glanceable companion, not the precision read: angle and arc are weaker channels than aligned length, so it pairs with `stacked_bar`/`table` for exact numbers. Needs an encoding-enum addition; the extreme seniority skew (senior vs. a handful of interns) means tiny categories need a minimum-size treatment or the chart hides its own headline.

### Milestone: Transport

- [ ] `chat-transport-and-tool-parts`
  Adopt a UI-layer streaming toolkit; keep OpenRouter as the provider. Cover the message-part model, partial tool-argument streaming, abort, transcript persistence and rehydration, and the split between server tools that compute over Postgres and client tools that touch composition state. Non-goals: the grammar and state semantics — their own specs.

### Milestone: The Surface

- [ ] `agent-readable-composition-state`
  Define how a rendered composition reaches the agent. Cover instance identity, the projection each composition emits — names a reader recognizes, not row IDs — read-on-send as the first coupling, and actions as the only write path. This carries scope across turns: "now do that by skill" resolves against the composition on screen. Selection state and deep-link params are the same values, stated once.

- [ ] `composition-frame-and-recovery`
  Define the chrome every composed analysis sits in. Cover what the user is shown about agent-visible state, recovery when the model composes something wrong or meaningless — recovery is recomposition — whether a composition in an old turn stays live or freezes, and streaming, empty, and degenerate results as designed states. The unhappy path is the product surface: the corpus guarantees sparse companies, failed runs, thin classified slices.

- [ ] `seed-compositions`
  The five single-measure catalog views as saved compositions. Each seeds a cold-start chip — a live-data answer the user did not type — and proves the grammar spans it. Movers, Demand Trend, Role Lifecycle at full corpus; Demand by Function, Seniority Mix behind the denominator. Company Profile (composite) and Skill Overlap (co-occurrence) wait on their primitives.

### Milestone: Rigor

- [ ] `demo-fixture-corpus`
  A seeded, deterministic slice that renders identically every run. Cover the degenerate shapes live data guarantees: a company with a handful of postings, a window with a failed run, a skill matching nothing. One source serving Storybook stories, composition evals, and recorded walkthroughs.

- [ ] `composition-quality-evals`
  Measure whether the grammar works and how prompting steers it. Cover a fixture question set, and composition quality scored on three axes — novel, sound, coverage-honest — per model across the OpenRouter catalog and across prompt variants. A rerunnable harness, so a prompt or description rewrite is scored, not argued.

## Epic: Profile-Led Exploration

Goal: A person onboards with a résumé, discovers roles related to their experience — including ones they have never heard named — pins the ones worth watching, and reads a dashboard that adapts to what is changing for those roles. Primary surface ahead of chat. Decisions: `../lib/project.md` §Settled architecture (profile, pins, dashboard). Items are in build order. Every item reading canonical roles or skills waits on current enrichment coverage.

### Milestone: Foundations

- [ ] `profile-and-pins`
  App-owned tables for one profile — past roles, claimed skills — and pinned canonical roles. First web-app write path, through Server Actions. Seed one real profile so later pages build before onboarding exists. A pin follows its role through taxonomy merge and retirement.

- [ ] `role-fit-measure`
  Move weighted skill coverage from the `persona-skill-gap` prototype into the measure engine: per-role demand weighted by requirement rate, the person's coverage of it, and the skills that stop and keep counting across a role change. The prototype computes it in-page, which the grammar forbids.

- [ ] `title-dimension-performance`
  `open_posting_titles` exceeds the MCP query timeout after the 2026-09-21 onboarding wave. Title heads label every pinned role, so this blocks Explore. Also decide whether title heads extend to closed postings; today they cover the open cohort only.

### Milestone: Discovery

- [ ] `explore-page`
  Suggest roles from the profile: the person's own roles, adjacent roles by skill coverage, and the title heads each role goes by. The person pins from here.

- [ ] `role-detail-page`
  Enough to decide whether to pin: title heads, required skills with the person's own marked, companies hiring, and trend.

### Milestone: Dashboard

- [ ] `dashboard-fixed-widgets`
  A fixed widget set per pinned role — arrivals trend, companies hiring, skills in demand, title heads. Learn which widgets earn a place before anything selects them.

- [ ] `resume-onboarding`
  Paste or upload a résumé; extract past titles and skills; match them to taxonomy terms; the person reviews every match before save. Unmatched items stay visible. Discloses the third-party extraction call. A cheap extraction spike can run earlier to test match quality.

- [ ] `widget-detectors`
  Deterministic detectors propose scored widget candidates from measure-engine output; a ranker chooses. Coverage- and failed-run-gated.

- [ ] `widget-narration`
  A model writes each widget's one-line reason from backend-supplied values. Choosing among detector candidates is a later step, evaluated against detector output.

## Epic: Job Classification

- [x] `codex-batch-enrich-runner`
  Done. Batch enrichment now defaults to a constrained, subscription-authenticated Codex runner. Claude remains an explicit fallback; Go retains selection, validation, writeback, provenance, and reporting.
