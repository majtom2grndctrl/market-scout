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

| Platform | Key | Coverage |
|---|---|---|
| Greenhouse | `internal_job_id` | 100% of 5,581 |
| Workday | `bulletFields[0]` (e.g. `R_105961`) | Already the URL identity; never fans out |
| Workable | `code` | Listing-level only |
| Lever, Ashby | `id` | 1:1 with posting |
| Gem | `id` | Zero postings in corpus — unverified |

Greenhouse `requisition_id` is unusable: NULL on 54 rows, and Stripe fills it with the literal `See Opening ID`.

Greenhouse fan-out corpus-wide: 5,109 requisition groups over 5,581 postings; **258 groups show fan-out, collapsing 452 rows (8.1% of Greenhouse postings).**

## Title hygiene

709 of 9,919 titles (7.1%) carry stray leading or trailing whitespace — 513 Greenhouse, 196 Ashby, zero elsewhere. 92 have internal double spaces. 128 use en or em dashes. **Zero** contain HTML entities, which is why entity normalization left scope.

Normalizing case and whitespace collapses 7,917 distinct titles to 7,784.

## Why not clean at ingest

`raw_data` is populated on 100% of snapshots, so a write-time field normalization would be recoverable in principle. But the only current `raw_data` reader (`migrations/000019_workplace_type_derivation.up.sql`) reads other keys, so that recoverability is untested. A `STORED GENERATED` column reaches the same clean-column outcome while re-deriving across all history whenever the expression changes — no version drift, nothing to backfill twice.

The deeper argument is that a rule baked into ingest versions across time. Fix it in November and May–October carries v1 while November onward carries v2, putting a step change in the trend line that came from code. The classifier already demonstrates this cost: `prompt_version` cohorts, trust tiers, the `data-audit` skill, and migration `000025` all exist because derived data got written into rows and the rules changed.

Row collapsing is worse than field normalization either way: a field rewrite leaves `raw_data` intact, while a row never written has no `raw_data` at all, and ATS boards do not serve history.

## Neither number is headcount

A Greenhouse job can carry multiple openings, and the public Job Board API does not expose that count. "Requisitions" means distinct jobs as the ATS identifies them, nothing more. The two counts also diverge only on Greenhouse today — Workday, Workable, Lever, and Ashby all key one posting per requisition — so collapsing to equal is expected, not a bug.
