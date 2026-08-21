# Measure Engine — research notes

Investigation behind `index.md`. Not a spec. Numbers are live snapshots, not contracts.

## Learn-checks (live DB, 2026-08-21)

Run via the `market-scout-postgres` read-only MCP.

| Question | Finding |
|---|---|
| Corpus | 119 companies, 8,618 postings, 70,628 snapshots, 1,707 success + 26 failed runs |
| History | 2026-05-15 → 2026-08-17; **16 distinct fetch days** (clustered, not daily) |
| Runs/company | mostly 13–22 successful runs |
| Cohorts | **open 4,595 · closed 4,023 · ever-seen 8,618** |
| Open classified | **24.0%** (1,103 / 4,595) — *lower* than the ~35% corpus-wide figure the grammar cites |
| Lifespan (closed) | median 21d, avg 28d, max 88d; **431 seen in exactly one run** |
| Timestamps | real time-of-day (`started_at`, `fetched_at`), one per run — not date-truncated |
| `rate` weekly | real gap: no week of 2026-07-13 (jumps 07-06 → 07-20); first weeks inflated by left-censoring |
| `delta` as-of | open 14d ago 4,436 → now 4,595; **as-of-now reconstruction == `open_postings` view exactly** |

## Load-bearing conclusions

1. **As-of-now open == `open_postings`.** The parameterized as-of query generalizes the read model's `open` definition; `open_postings` is its `ts = now` special case. Argues for defining openness once in SQL and letting the engine read it, never reproduce it.
2. **Classified coverage is cohort-specific.** Open is 24%, not 35%. A single corpus-wide denominator would misstate coverage. Denominator must be computed against the composition's cohort.
3. **Weekly-only is honest; daily is not.** 16 fetch days across 3 months. Daily buckets would be mostly empty.
4. **Gaps are real, not hypothetical.** A week with no successful run in scope must render as a gap, never an interpolated point. Live data already contains one (07-13).
5. **Single-run postings exist (431).** Lifespan of a posting seen in one run only needs a defined rule (span 0, or excluded).
6. **`open_posting_taxonomy` is open-only.** It joins `open_postings_display`. Closed/all cohort groupings by role/skill/etc. have no taxonomy source today — the migration must supply a cohort-agnostic one.

## Reference queries

As-of open (delta's core), proven to reproduce `open_postings` at `ts = now`:

```sql
-- per company, latest success run with started_at <= D; postings in that run
WITH latest AS (
  SELECT DISTINCT ON (fr.company_id) fr.company_id, fr.id AS fetch_run_id
  FROM fetch_runs fr
  WHERE fr.status = 'success' AND fr.started_at <= $D
  ORDER BY fr.company_id, fr.started_at DESC, fr.id DESC
)
SELECT DISTINCT ps.job_posting_id
FROM latest l JOIN posting_snapshots ps ON ps.fetch_run_id = l.fetch_run_id;
```

Lifespan primitive (first/last success-run appearance per posting):

```sql
SELECT ps.job_posting_id,
       min(fr.started_at) AS first_seen,
       max(fr.started_at) AS last_seen
FROM posting_snapshots ps
JOIN fetch_runs fr ON fr.id = ps.fetch_run_id AND fr.status = 'success'
GROUP BY ps.job_posting_id;
```
</content>
