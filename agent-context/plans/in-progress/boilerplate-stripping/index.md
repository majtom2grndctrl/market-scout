# Boilerplate Stripping

## Goal

Strip per-company boilerplate (company blurbs, EEO statements, recruiter pitches) from `posting_snapshots.description_text` before the `/batch-enrich` skill ships it to a Haiku agent. Cuts LLM input tokens and concentrates role-specific signal for both classification and the embedding-target summary.

## Scope

### In scope

- New Go package exposing a pure-function helper that, given a company's posting descriptions, returns the same descriptions with shared boilerplate removed.
- One sqlc query that returns the latest snapshot's `description_text` per `job_posting_id` for a given `company_id`.
- A wiring step in `.claude/skills/batch-enrich/SKILL.md` after taxonomy load (step 4) and before dispatch (step 5) that calls the helper per company and substitutes the cleaned text into the agent prompt.
- Unit tests covering the cases listed under Acceptance.

### Out of scope (non-goals)

- Caching detected boilerplate on `companies` or any new table. Compute fresh each enrichment run.
- Schema changes — no new tables, no new columns. Snapshot rows are never rewritten.
- Section-based extraction (pull "Responsibilities" / "Requirements" specifically). Different approach.
- LLM-based cleaning or pre-pass classification.
- Cross-company boilerplate (e.g., generic EEO text shared across employers). Per-company is enough for v1.
- Bootstrap optimization for new companies (companies with <3 postings just pass through).

## Acceptance criteria

- [ ] A Go helper accepts N descriptions for one company and returns N cleaned descriptions in the same order. No DB writes; no I/O beyond what the caller passes in.
- [ ] When N < 3, every output equals its input byte-for-byte.
- [ ] When ≥60% of inputs share a verbatim paragraph (or contiguous run of paragraphs) of ≥200 characters, that paragraph block is absent from every output that contained it, except where empty-residue safety applies (see bullet above).
- [ ] When inputs share no run meeting the length and prevalence thresholds, every output equals its input.
- [ ] When the entire description is itself shared boilerplate, no posting is reduced to empty/whitespace — if removal would empty an input, that input is returned unchanged. See Rough sketch for the safety rule.
- [ ] A new sqlc query returns the latest snapshot's `description_text` per `job_posting_id` for a given `company_id`, skipping rows where `description_text IS NULL`.
- [ ] The `/batch-enrich` skill calls the helper per company after selection and before dispatch, and the agent prompt receives the cleaned text. Selection still reads from `posting_snapshots` as today; the canonical raw row is untouched.

**Performance note (informational, not a gate):** Helper should complete well under a second for a company with ≤100 postings of typical length (a few KB each) on a developer laptop.

## Rough sketch

### Package and signature

New package `internal/enrich/boilerplate`. (No existing `internal/enrich/` tree today — `internal/` currently holds `ats/`, `db/`, `domain/`. The helper does not belong in `ats/` (HTTP only, no enrichment concerns) or `db/` (sqlc output). A new top-level enrichment package is the natural home and leaves room for sibling helpers later.)

Proposed exported function:

```go
// Proposed design — internal/enrich/boilerplate/strip.go
package boilerplate

// Strip removes per-company boilerplate shared across descriptions.
// Returns one cleaned string per input, in the same order. Pure; no I/O.
// Inputs with fewer than minSamples entries (see package defaults) are
// returned unchanged.
func Strip(descriptions []string) []string
```

Tuning knobs live as package-level constants for v1. The implementing agent picks defaults and adjusts based on test outcomes:

- `minSamples` — minimum descriptions required to attempt detection. Suggested: 3.
- `minMatchLen` — minimum match length in bytes. Suggested: 200.
- `minPrevalence` — minimum fraction of inputs a match must appear in. Suggested: 0.6.

If tuning needs to vary per call site later, promote constants to a `StripOptions` struct without breaking the default `Strip` signature.

### Algorithm contract

The contract — not the algorithm. Two implementers should produce equivalent observable behavior.

- **Match unit:** contiguous byte substrings of the input, anchored on paragraph boundaries. Normalization before splitting: convert line endings to `\n`; treat any run of ≥2 newlines (with only whitespace between) as a paragraph boundary. Whole paragraphs only; sub-paragraph fragments are ignored. Match comparison is byte-exact on normalized paragraphs — normalization is limited to the line-ending and paragraph-boundary rules above; intra-paragraph whitespace is not further normalized before comparison. If real-world hit rates suffer due to whitespace artifacts in HTML-derived text, revisit before shipping. Rationale: boilerplate appears as whole paragraphs (company blurbs, EEO statements); paragraph anchoring avoids cutting role text mid-sentence and dramatically narrows the search space.
- **Length filter:** match length ≥ `minMatchLen` bytes (Go `len`).
- **Prevalence filter:** match appears verbatim in ≥ `ceil(minPrevalence * N)` of the N inputs (counted by input, not by occurrence).
- **Overlap handling:** when one qualifying match contains another, prefer the longest. When two qualifying matches partially overlap, remove both — collect all qualifying match ranges, sort by start offset, merge overlapping intervals transitively (any set of qualifying matches where each overlaps with at least one other in the set is treated as a single removal range), then remove the merged ranges in one pass. Disjoint qualifying matches are all removed; the longest-preference rule applies only to overlapping or nested matches.
- **Removal:** strip every qualifying match from every input it appears in. After removal, collapse any run of ≥2 consecutive blank lines to a single blank line, and trim leading and trailing whitespace from the result. Do not otherwise modify surviving paragraphs. Apply this whitespace normalization only when at least one removal occurred; inputs with no qualifying matches are returned byte-for-byte unchanged.
- **Empty-residue safety:** if removal would reduce any input to an empty or whitespace-only string, that input is returned unchanged. Better to ship boilerplate to the agent than to ship nothing — the AC requires no posting becomes empty.

The implementing agent picks the algorithm (suffix array, rolling-hash multiset, line-set intersection, etc.). Paragraph anchoring makes a hash-based set intersection over paragraphs the obvious starting point, but that is not part of the contract.

### DB read shape

Add to `internal/db/queries/fetcher.sql`:

```sql
-- name: ListLatestDescriptionsByCompany :many
-- Latest snapshot's description_text per job_posting for a company.
-- Skips postings whose latest snapshot has NULL description_text.
```

Implementer writes the query body — a `DISTINCT ON (ps.job_posting_id)` ordered by `ps.fetched_at DESC`. `company_id` lives on `job_postings`, not `posting_snapshots`, so the query must JOIN `posting_snapshots ps` to `job_postings jp ON jp.id = ps.job_posting_id` and filter by `jp.company_id = $1` and `ps.description_text IS NOT NULL`, returning `(ps.job_posting_id, ps.description_text)`. Re-run `sqlc generate`; never hand-edit `internal/db/fetcher.sql.go`. If the latest snapshot for a posting has NULL `description_text`, that posting is omitted entirely — no fall-through to earlier snapshots.

The runtime caller is `cmd/strip-boilerplate` (see Skill integration). The sqlc query is the single source of truth; the binary uses the generated code directly. The skill does not issue the corpus query via psql. Note: because `posting_snapshots.description_text` is nullable in the schema, sqlc will type the column as `sql.NullString` in the generated row type even with the `IS NOT NULL` filter (sqlc does not infer non-null from WHERE clauses). Use `ps.description_text::text` in the SELECT list to force a non-null `string` type in generated code; this is simpler than converting `sql.NullString` in the binary.

### Skill integration

Edit `.claude/skills/batch-enrich/SKILL.md`. Insert a new step after step 4 (Load existing taxonomy) and before step 5 (Dispatch waves), renumbering as needed. Also extend the step 3 selection query to return `company_id` alongside `posting_id`, `title`, and `description_text` — grouping by company in the new step requires it.

> **Strip per-company boilerplate.** Group selected postings by `company_id`. For each company with ≥3 selected postings, pipe `{"company_id": <int>, "selected_ids": [<int>, …]}` to `cmd/strip-boilerplate` on stdin. The binary self-fetches that company's full latest-description corpus from the DB, runs it through `boilerplate.Strip`, and writes `{"postings": [{"posting_id": <int>, "cleaned_text": "<string>"}, …]}` to stdout. The binary emits one entry per `selected_id`; if the full corpus has fewer than `minSamples` entries (so `Strip` passes through unchanged), `cleaned_text` is the original description verbatim. The skill substitutes `cleaned_text` for `description_text` in the agent prompt unconditionally — no fallback branch needed. Companies with <3 selected postings skip the binary call entirely and pass through unchanged. If the binary exits non-zero, abort the skill. Errors to stderr.

`cmd/strip-boilerplate` opens its own DB connection via `DATABASE_URL` (same pattern as `cmd/fetcher` and `cmd/migrate`) and uses the sqlc-generated `ListLatestDescriptionsByCompany` to fetch the full corpus — all of the company's latest descriptions, not just the batch selection. Wider corpus improves detection accuracy. The canonical `posting_snapshots.description_text` row is never written.

### Tests

Unit tests in `internal/enrich/boilerplate/strip_test.go`. Black-box (`package boilerplate_test`) where it doesn't force exporting internals.

Cases:

- **Abundant samples, clear boilerplate.** N=5 inputs sharing a 4-paragraph company blurb plus distinct role bodies. Assert: blurb absent from every output; role-body paragraph contents preserved verbatim (inter-paragraph whitespace may be normalized).
- **Below bootstrap threshold.** N=2 with obvious shared text. Assert: outputs equal inputs.
- **Diverging samples.** N=8 with no shared paragraph meeting both filters. Assert: outputs equal inputs.
- **Entire description is boilerplate.** One input in the set is exactly the shared blurb with nothing else. Assert: that input is returned unchanged (empty-residue safety); others have the blurb stripped.
- **Multiple disjoint boilerplate blocks.** Inputs share two separate qualifying blocks (e.g., a company blurb at the top and an EEO footer). Assert: both stripped.
- **Order preservation.** Outputs match inputs index-for-index.

DB integration test in `internal/db/` (build tag `//go:build integration`) for `ListLatestDescriptionsByCompany`: insert two postings for one company with multiple snapshots each (one snapshot has NULL `description_text`); assert the query returns the latest non-null per posting, skips the NULL-tail posting if its latest is NULL.

## Sequencing

**Phase 1 (concurrent):** Helper package + tests (`internal/enrich/boilerplate`); sqlc query addition + integration test (`internal/db/queries/fetcher.sql`). Independent files, independent tests.
**Phase 2 (sequential):** `cmd/strip-boilerplate` binary + skill edit (`.claude/skills/batch-enrich/SKILL.md`). Binary consumes the `boilerplate.Strip` signature and `ListLatestDescriptionsByCompany` query name from Phase 1. The step-3 selection query change (add `company_id` to the SELECT list) is a prerequisite for the skill edit but has no Phase 1 dependency — it can be done at the start of Phase 2.

## Open questions

- Is paragraph anchoring tight enough, or do we want sentence-level anchoring as a fallback for descriptions that are one giant paragraph? Defer until a real failure case appears — the 5-posting Cognition sample showed paragraph-anchored boilerplate, which is the v1 target.
- `minMatchLen=200` and `minPrevalence=0.6` are guesses calibrated to the Cognition observation. Implementer should sanity-check on at least one other watchlist company's snapshots before locking defaults.
- With `minSamples=3` and `minPrevalence=0.6`, only 2 of 3 inputs need to share a paragraph for detection to fire. This is intentionally aggressive for small companies just past the bootstrap threshold; raising `minPrevalence` to 0.75 or `minSamples` to 5 is a valid callout if false positives appear during implementation.
