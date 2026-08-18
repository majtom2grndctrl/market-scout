# UI View Catalog

> **⚠️ Superseded — read `composition-grammar.md` first.** The product model here — seven fixed views as the deliverable — is replaced by the composition grammar. What stays valid: the corpus figures, coverage split, shared params, and out-of-scope list below. The seven views survive only as seed compositions.
>
> **Purpose:** Candidate view set for an AI-first Market Scout UI, scored against what the data can actually support.
> **Sources:** Live queries against the local Postgres instance, July 2026. Data range 2026-05-15 → 2026-07-20.
> **Status:** Superseded in framing by `composition-grammar.md`; data constraints still current. Numbers are a July snapshot — re-measure before committing.

---

## Product Frame

Chat is the front door. A user arrives, asks a question, and gets back prose, a data visualization, and a link to the page holding the authoritative records behind the claim.

## Design Contract

Each view is one tool, one chart, one page route. Four artifacts per view:

| Artifact | Role |
|---|---|
| Tool definition | What the chat model may call. Typed params. |
| Result shape | Typed rows, not prose. |
| Chart component | Deterministic render of that shape. |
| Page route | Same query, full and paginated. |

Two consequences make this worth the constraint:

**The deep link is the tool call serialized.** A call to a demand-trend tool scoped to one company over ten weeks renders a chart in chat and links to the equivalent company trend page carrying the same params. The link is derived, never hand-authored. Page and chat answer cannot disagree, because they are the same query at two altitudes.

**The model never emits a number.** It emits prose about numbers the tool returned and the chart rendered. Fabricated figures become structurally impossible rather than prompt-discouraged.

The alternative — text-to-SQL over the schema — was rejected. It yields unverifiable numbers and no natural deep link, against a product whose value is authoritative intel.

## View Catalog

| View | Answers | Chart form | Backing | Coverage |
|---|---|---|---|---|
| Demand Trend | Who is ramping? One company vs. another over time. | Line/area, weekly points | Snapshots + fetch runs | Full corpus |
| Movers | Who moved up or down this window? | Diverging ranked bars | Snapshot deltas | Full corpus |
| Role Lifecycle | How fast do roles fill? Evergreen vs. scramble hires? | Histogram + open/closed split | First-seen + absence | Full corpus |
| Company Profile | What is this company staffing right now? | Composite scorecard | Snapshots + department | Full corpus |
| Seniority Mix | Who hires junior people? | Stacked composition bar | Classification seniority | Classified subset |
| Demand by Function | Engineering vs. Sales vs. Design demand. What skills are asked for? | Ranked bars with denominator | Role dimensions, skills | Classified subset |
| Posting Explorer | Drill-down destination for every claim. | Faceted table | All postings | Full corpus |

The four full-corpus views are the launch set. They carry the differentiated story and depend on no enrichment. The two classified views ship behind a coverage denominator. The Explorer is the trust anchor.

### Role Lifecycle

The view a scraped job board structurally cannot build, and the strongest candidate to lead with.

Posting longevity is bimodal, not uniformly churny. A short-lived layer sits on top of a large persistent core: 688 closed postings died inside a week, while 1,843 currently-open postings have been open more than six weeks. Both populations are real. The split is the finding.

Trustworthiness rests on fetch-run-aware absence. A posting counts as closed only when its company's most recent run succeeded. A failed run or an unfetched company is distinguishable from a genuinely removed posting. Without that distinction the metric is a guess.

Grouping by company and role turns it into comparison: research roles staying open far longer than sales roles at the same company is a claim this data supports.

### Company Profile

The natural landing target when the chat mentions a company, and the drill-through from Movers. A composite rather than a single chart: demand sparkline, current open count, top departments, top roles.

Department is safe to feature at 92% snapshot coverage.

### Movers

Best opening view for a cold visitor. An AI-first landing page with an empty chat box gives a user with no question nothing to do. Seeding the landing with live movers as clickable chips — each chip a pre-baked tool call — answers a question the user did not have to type.

### Posting Explorer

Every chat answer ends in a link to records, and they all funnel here: one filterable table over all 6,969 postings. This is what makes numbers auditable. A claim of N open roles in some category links to a filtered table showing those N rows.

It also lets the classified-gated views degrade gracefully. At partial coverage the Explorer behind them still shows real postings — just fewer carrying role tags.

## Shared Parameters

Two params recur across views. Designing them as shared types is the difference between one system and seven one-offs.

**Scope** — all companies, one company, one role, one skill. Composition falls out of it: movers restricted to design roles is the movers tool with a scope argument, not a new view.

**Window** — the time range for trend and movers. Only 12 distinct fetch days exist across a ten-week span. Weekly buckets read honestly; daily buckets imply granularity the data lacks.

**Coverage denominator** — a shared UI element, not a per-view caption. Classified views render the denominator inline, and the tool returns it in the result so the chat narrates it aloud rather than implying full coverage. A dashboard can disclose partial coverage in a footnote. A bot asserting a market-wide skill ranking from a third of the corpus is simply wrong, confidently.

## Data Constraints

Measured July 2026. Re-verify before planning.

| Asset | Value |
|---|---|
| Companies | 114 |
| Job postings | 6,969 |
| Snapshots | 52,746 across 12 fetch days |
| Description coverage | 6,601 of 6,969 (95%) |
| Classified postings | ~2,224 (32%) |
| Fetch runs | 1,261 success / 14 failed |
| Open vs. closed | 4,335 open / 2,634 closed |
| Closed posting lifespan | Median 18 days, mean 23 |
| Open posting age | Median 35 days |

Density is concentrated: Stripe 887 postings, Anthropic 735, OpenAI 720, Harvey 543. Company-scoped views should be designed against these and checked against the thin tail.

Seniority is non-null on every classification, so it is fully covered within the classified subset. The distribution is itself a finding — 1,238 senior against 105 junior and 9 intern. Scarcity is the answer to the junior-hiring question.

## Out of Scope

Steer users away from these. Each is a data gap, not a backlog item.

- **Compensation.** Structured pay exists on 117 postings (1.7%), effectively Lever boards only. Any comp chart renders noise. The right response to a pay question is a genuine refusal, which bounded tools give for free. Extracting pay from description HTML is a parsing project, not a UI project.
- **Geography.** The posting city array is empty on all 6,969 rows. Only free-text location strings exist. Location answers would be parsed on the fly and wrong.
- **Remote vs. onsite.** Workplace type is present on 30% of snapshots, and which platforms report it is non-random. The observed hybrid/remote/onsite split describes ATS reporting habits more than the market.
- **Whole-market claims.** 114 hand-picked, AI-heavy companies. Sound for "this niche," misleading as "tech hiring."
- **Causality and personal advice.** Why a company is hiring, and whether a user is qualified, are both outside what postings support.

## Open Questions

- Chat as landing experience with pages as destinations, versus a persistent panel beside a browsable app. Framing so far assumes the former.
- Enrichment cadence, deferred to its own session. Coverage of open postings sits near 23% against 47% for closed. This is a cohort artifact of a single enrichment pass over the May population, not a bias toward short-lived postings — median posting age at classification was 5 days, and those postings stayed observable for 48 days after. June and July cohorts sit at zero.
- The chat layer is the project's first runtime model-API dependency. Batch enrichment deliberately runs through subscription-authenticated CLI subprocesses with no SDK or REST path. A web request cannot shell out to a subscription CLI, so chat implies an API key and per-question cost. Defensible, but a departure from a settled decision and worth logging as one.
- An AI-first UI un-defers two current non-goals: Next.js product screens and the agent UI. Both need updating when this is committed to.
