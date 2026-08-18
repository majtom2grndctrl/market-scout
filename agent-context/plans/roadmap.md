# Plans Roadmap

> **Read this when:** choosing the next plan to draft, review, promote, or orchestrate.
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

- [ ] `composition-grammar`
  Define the vocabulary and its invariant. Four slots — measure (open count, delta, age, lifespan, rate, share), grouping (company, role, specialization, skill, seniority, function, weekly bucket), filter, encoding — plus fetch-run-aware absence as a modifier on every count and time measure. Backend computes every primitive; the model selects and combines, never calculates. Unreliable data types are absent from the vocabulary, not refused per question — no compensation measure, no geography grouping — so out-of-scope is structural. A composition serializes to the deep link and back. Versioned as a public contract.

- [ ] `measure-engine`
  Compute a serialized composition into typed rows plus a coverage denominator. Lives in `apps/web/lib/db`, over the read-model views. Owns delta, lifespan, rate, and share on top of the open/closed definition the read model already fixes. Classified groupings return their denominator inline, so a chart over the 35% classified slice cannot imply full coverage. Never hands raw SQL to the model.

- [ ] `chart-primitives`
  Define the chart layer. `d3-scale`, `d3-shape`, `d3-array` supply scales and path geometry; the app owns every mark, so charts inherit design tokens rather than a library theme. Three layers — dimensionless shaping, a composition layer owning scales and margins, and primitives that receive scales as props and never build their own. Render target is any shape the grammar produces, not a fixed set of forms. Settle the fixed-`viewBox` default and what earns a measured wrapper, UTC scales so server and client render identical ticks, and a failed-run window as a gap, never an interpolated line. Absence is data; a chart that smooths over it lies.

### Milestone: Transport

- [ ] `chat-transport-and-tool-parts`
  Adopt a UI-layer streaming toolkit; keep OpenRouter as the provider. Cover the message-part model, partial tool-argument streaming, abort, transcript persistence and rehydration, and the split between server tools that compute over Postgres and client tools that touch composition state. Non-goals: the grammar and state semantics — their own specs.

### Milestone: The Surface

- [ ] `agent-readable-composition-state`
  Define how a rendered composition reaches the agent. Cover instance identity, the projection each composition emits — names a reader recognizes, not row IDs — read-on-send as the first coupling, and actions as the only write path. This carries scope across turns: "now do that by skill" resolves against the composition on screen. Selection state and deep-link params are the same values, stated once.

- [ ] `composition-frame-and-recovery`
  Define the chrome every composed analysis sits in. Cover what the user is shown about agent-visible state, recovery when the model composes something wrong or meaningless — recovery is recomposition — whether a composition in an old turn stays live or freezes, and streaming, empty, and degenerate results as designed states. The unhappy path is the product surface: the corpus guarantees sparse companies, failed runs, thin classified slices.

- [ ] `seed-compositions`
  The seven catalog views as saved compositions. Each seeds a cold-start chip — a live-data answer the user did not type — and proves the grammar spans it. Movers, Demand Trend, Role Lifecycle, Company Profile at full corpus; Demand by Function, Seniority Mix, Skill Overlap behind the denominator.

### Milestone: Rigor

- [ ] `demo-fixture-corpus`
  A seeded, deterministic slice that renders identically every run. Cover the degenerate shapes live data guarantees: a company with a handful of postings, a window with a failed run, a skill matching nothing. One source serving Storybook stories, composition evals, and recorded walkthroughs.

- [ ] `composition-quality-evals`
  Measure whether the grammar works and how prompting steers it. Cover a fixture question set, and composition quality scored on three axes — novel, sound, coverage-honest — per model across the OpenRouter catalog and across prompt variants. A rerunnable harness, so a prompt or description rewrite is scored, not argued.

## Epic: Job Classification

- [x] `codex-batch-enrich-runner`
  Done. Batch enrichment now defaults to a constrained, subscription-authenticated Codex runner. Claude remains an explicit fallback; Go retains selection, validation, writeback, provenance, and reporting.
