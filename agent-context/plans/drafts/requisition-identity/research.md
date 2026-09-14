# Research — Requisition Identity

Measurements behind the spec's decisions. Taken 2026-09-10 against `market_scout`, vitest fixture companies excluded (`board_token ~ 'vitest|^secondary-|^empty-|^no-runs-'`).

## What prompted this

Per-company posting counts looked wrong. Snap! Raise showed 131 open postings with 86% still open, which read as either explosive growth or a stale board. Neither: it posts one sales role per US city.

## Fan-out is three different phenomena, not one

| Pattern | Example | Signal |
|---|---|---|
| ATS-native fan-out — one requisition listed many times | Boulder Care, 49 postings under Greenhouse req `4458330008` | `internal_job_id` |
| Template fan-out — many requisitions sharing one description | Snap! Raise, 153 postings with **153 distinct** `internal_job_id` | description hash |
| Native multi-location — one posting covering many places | Ashby `secondaryLocations`, non-empty on 563 of 3,177 | already one row |

Only the first is a miscount. Snap! Raise genuinely holds 153 separate requisitions; the discomfort there is comparability between commission-only territory reps and engineering reqs, which `filter:function` answers once classification coverage exists.

## Why a description hash cannot anchor identity

**3,690 of 9,256 postings with descriptions (39.9%) changed description at least once during their observed life.** A hash-keyed group changes identity for two postings in five, so any time-series grouping built on it would show movement that came from ATS copy edits rather than the market. This killed the original plan, which keyed groups on `(company_id, normalized_title, description_hash)`.

That key also failed on its own motivating cases:

- ElevenLabs: 417 postings, **411 distinct titles** — the country sits inside the title (`Deployment Strategist - Brazil`), so the key collapses nothing.
- Boulder Care: the 49-posting requisition splits across 3 description hashes and 2 titles.

## Requisition key availability

First pass, superseded in detail by *Wire-key reality per platform* below — which corrects the Greenhouse and Workable coverage figures after reading the adapters and re-querying.

Greenhouse `requisition_id` is unusable: NULL on 57 postings, and Stripe fills it with the literal `See Opening ID`. `internal_job_id` is the usable key.

Greenhouse fan-out corpus-wide: 5,109 requisition groups over 5,581 postings; **258 groups show fan-out, collapsing 452 rows (8.1% of Greenhouse postings).**

## Title hygiene

709 of 9,919 titles (7.1%) carry stray leading or trailing whitespace — 513 Greenhouse, 196 Ashby, zero elsewhere. 92 have internal double spaces. 128 use en or em dashes. **Zero** contain HTML entities, which is why entity normalization left scope.

Normalizing case and whitespace collapses 7,917 distinct titles to 7,784.

## Why not clean at ingest

This mattered while the plan still carried a title-normalization step. It no longer does, but the reasoning governs any future proposal to clean at ingest, so it is recorded here.

`raw_data` is populated on 100% of snapshots, so a write-time field normalization would be recoverable in principle. But the only current `raw_data` reader (`migrations/000019_workplace_type_derivation.up.sql`) reads other keys, so that recoverability is untested. Where a clean column is genuinely wanted, a `STORED GENERATED` column reaches the same outcome while re-deriving across all history whenever the expression changes — no version drift, nothing to backfill twice.

The deeper argument is that a rule baked into ingest versions across time. Fix it in November and May–October carries v1 while November onward carries v2, putting a step change in the trend line that came from code. The classifier already demonstrates this cost: `prompt_version` cohorts, trust tiers, the `data-audit` skill, and migration `000025` all exist because derived data got written into rows and the rules changed.

Row collapsing is worse than field normalization either way: a field rewrite leaves `raw_data` intact, while a row never written has no `raw_data` at all, and ATS boards do not serve history.

## Neither number is headcount

A Greenhouse job can carry multiple openings, and the public Job Board API does not expose that count. "Requisitions" means distinct jobs as the ATS identifies them, nothing more. The two counts also diverge only on Greenhouse today — Workday, Workable, Lever, and Ashby all key one posting per requisition — so collapsing to equal is expected, not a bug.

## Requisition key stability

Greenhouse `internal_job_id` changed on **0 of 4,992** postings observed across more than one snapshot — 0.00%. For contrast, `source_first_published_at` was rewritten mid-life on 51.5% of Workday postings and 9.5% of Ashby postings.

So the architecture's distrust of ATS-reported fields is specific to timestamps that refresh on repost. An identity key is empirically a different class of signal.

## Why repost detection cannot be measured yet

Searching for repost signatures — a closed posting followed by a new `source_url` under the same requisition key — returns 889 pairs across 53 requisitions and 12 companies. **This measurement is confounded and should not be cited.** A fan-out group produces closed-then-reopened pairs whenever a territory closes and another opens, and Boulder Care's 49-posting requisition manufactures hundreds on its own.

Separating repost from fan-out requires the requisition key this spec delivers. The question is sequenced after this work, not deferred by neglect.

## Why template fan-out left scope

The only company it explains is Snap! Raise, whose posting and requisition counts already agree (153 and 153). The pair discharges the honesty contract; a third number would narrate a gap that does not exist. The residual discomfort — commission-only territory reps counted beside engineering requisitions — is a `filter:function` question, not a duplication one.

Dropping it also dropped `normalized_title`, which had no other consumer: title is not a member of `GROUPINGS` in `apps/web/lib/composition/vocabulary.ts`. The 709 stray-whitespace titles remain cosmetic until something groups on title.

## Wire-key reality per platform

Measured 2026-09-14. Only three platforms expose an identifier distinct from posting identity:

| Platform | Key | Reality |
|---|---|---|
| Greenhouse | `internal_job_id` | Non-null number on 99.3% of snapshots. **Present-but-JSON-null on 356 snapshots across 20 postings** — must land NULL, not `"0"`. |
| Workday | `bulletFields[0]` | Present on 100%. `BulletFields` already parsed on `wdJob` but unread. |
| Workable | `code` | Non-empty on **20 of 498** snapshots (96% NULL). A further 66 carry it as an empty string. |
| Ashby, Lever, Gem | — | `job.ID` is already `SourceID` (`ashby.go:149`, `lever.go:213`, `gem.go:154`). No distinction to record. |

Gem boards are Greenhouse-shaped and carry `internal_job_id` in their payload, so any backfill that sniffs `raw_data` for the key instead of branching on `companies.ats` would wrongly populate Gem rows.

## Requisition keys are board-scoped, not global

Greenhouse ids are board-scoped, Workday's tenant-scoped, and Workable `code` is human-typed — live values are `2026-37`, `2026-92`, `SQ204`. Zero cross-company collisions exist among the current 5,700 keys, but two Workable boards numbering by year-week collide on the first overlap. An ungrouped `count` has no company boundary of its own, so the view exposes `company_id` and consumers count distinct over the pair.

## Why the current-snapshot lateral must come from 000019

`000017_read_model_views.up.sql` resolves the current snapshot with `WHERE job_posting_id = ... ORDER BY fetched_at DESC LIMIT 1` — no run predicate. `000019_workplace_type_derivation.up.sql` added `AND posting_snapshots.fetch_run_id = open_postings.fetch_run_id` plus an `id DESC` tie-breaker, with a comment explaining that without it "a snapshot written by a later failed run — or by no run at all — could supply the displayed row."

Copying the older shape reproduces the bug 000019 fixed, and a fixture test stays green either way.

## Why requisitions stop at the open cohort

`count` serves `open`, `closed`, and `all`. `posting_requisitions` derives from `open_postings`, so under the other two an inner join would drop rows and change `value` — the posting count itself — while a left join would report 0 requisitions as though measured. Closed postings would need a non-run-scoped resolution, which is exactly the derivation the run predicate above exists to avoid trusting. The cohorts carry no `requisitions` key instead.

Observed fan-out on the open cohort, for fixture sizing: Anthropic 600 postings / 561 requisitions, Stripe 614/592, Scale AI 219/199, Glean 112/85, Boulder Care 19/8.

## Fan-out value per platform

Measured 2026-09-14 over current snapshots of all postings. Collapses = postings carrying a key, minus distinct `(company_id, key)` pairs.

| Platform | Postings | Carry a key | Distinct keys | Collapses |
|---|---|---|---|---|
| Greenhouse | 5,581 | 100% | 5,109 | 452 |
| Workday | 594 | 100% | 588 | 6 |
| Workable | 56 | 5 (9%) | 3 | 2 |

Greenhouse is roughly 98% of the value this spec delivers. Workday and Workable each still produce real collapses from honest platform keys, so both stay in — the cost is a struct field and a backfill branch each.

Workable's keys live entirely on the `seeq` board: codes `2026-37`, `2026-92`, `SQ204` across 5 of its 51 postings, two sharing a code. The other Workable board (`vouched`) populates none. That makes `seeq` the fixture source for the non-empty half of the Workable extraction rule — the half that produces the collapse, and the half no existing fixture covers.
