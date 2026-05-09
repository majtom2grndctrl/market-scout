# Boilerplate Stripping

## Goal

Strip per-company boilerplate (company blurbs, EEO statements, recruiter pitches) from `posting_snapshots.description_text` before the `/batch-enrich` skill ships it to a Haiku agent. Cuts LLM input tokens and concentrates role-specific signal for both classification and the embedding-target summary.

## Scope

### In scope

- New Go package exposing a pure-function helper that, given a company's posting descriptions, returns the same descriptions with shared boilerplate removed.
- One sqlc query that returns the latest snapshot's `description_text` per `job_posting_id` for a given `company_id`.
- A wiring step in `.claude/skills/batch-enrich/SKILL.md` between selection (step 3) and dispatch (step 5) that calls the helper per company and substitutes the cleaned text into the agent prompt.
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
- [ ] When ≥60% of inputs share a verbatim run of ≥200 characters, that run is absent from every output that contained it.
- [ ] When inputs share no run meeting the length and prevalence thresholds, every output equals its input.
- [ ] When the entire description is itself shared boilerplate, the helper leaves at least a non-empty residue per posting (a posting is never reduced to empty/whitespace) — see Rough sketch for the safety rule.
- [ ] A new sqlc query returns the latest snapshot's `description_text` per `job_posting_id` for a given `company_id`, skipping rows where `description_text IS NULL`.
- [ ] The `/batch-enrich` skill calls the helper per company after selection and before dispatch, and the agent prompt receives the cleaned text. Selection still reads from `posting_snapshots` as today; the canonical raw row is untouched.
- [ ] Helper completes in well under a second for a company with ≤100 postings of typical length (a few KB each) on a developer laptop. Order-of-magnitude target only — no benchmark gate.

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
- `minMatchLen` — minimum match length in characters. Suggested: 200.
- `minPrevalence` — minimum fraction of inputs a match must appear in. Suggested: 0.6.

If tuning needs to vary per call site later, promote constants to a `StripOptions` struct without breaking the default `Strip` signature.

### Algorithm contract

The contract — not the algorithm. Two implementers should produce equivalent observable behavior.

- **Match unit:** contiguous byte substrings of the input, anchored on paragraph boundaries (`\n\n` or equivalent blank-line break after whitespace normalization). Whole paragraphs only. Sub-paragraph fragments are ignored. Rationale: boilerplate appears as whole paragraphs (company blurbs, EEO statements); paragraph anchoring avoids cutting role text mid-sentence and dramatically narrows the search space.
- **Length filter:** match length ≥ `minMatchLen` characters.
- **Prevalence filter:** match appears verbatim in ≥ `ceil(minPrevalence * N)` of the N inputs (counted by input, not by occurrence).
- **Overlap handling:** when one qualifying match contains another, prefer the longest. When two qualifying matches partially overlap, both may be removed independently — they were each computed against the input, not against each other's residue.
- **Removal:** strip every qualifying match from every input it appears in. Collapse the resulting whitespace/blank-line runs so the cleaned text reads naturally.
- **Empty-residue safety:** if removal would reduce any input to an empty or whitespace-only string, that input is returned unchanged. Better to ship boilerplate to the agent than to ship nothing — the AC requires no posting becomes empty.

The implementing agent picks the algorithm (suffix array, rolling-hash multiset, line-set intersection, etc.). Paragraph anchoring makes a hash-based set intersection over paragraphs the obvious starting point, but that is not part of the contract.

### DB read shape

Add to `internal/db/queries/fetcher.sql`:

```sql
-- name: ListLatestDescriptionsByCompany :many
-- Latest snapshot's description_text per job_posting for a company.
-- Skips postings whose latest snapshot has NULL description_text.
```

Implementer writes the query body — a `DISTINCT ON (job_posting_id)` ordered by `fetched_at DESC`, filtered by `company_id` and `description_text IS NOT NULL`, returning `(job_posting_id, description_text)`. Re-run `sqlc generate`; never hand-edit `internal/db/fetcher.sql.go`.

The `/batch-enrich` skill uses psql directly (it doesn't link the Go binary), so the runtime caller is the skill's psql block, not generated Go code. The sqlc query is added so any future Go-side caller has a single source of truth, and so the SQL is reviewed in one place. The skill's psql call mirrors the sqlc query body.

### Skill integration

Edit `.claude/skills/batch-enrich/SKILL.md`. Insert a new step between current step 3 (Select postings) and step 5 (Dispatch waves), renumbering as needed:

> **Strip per-company boilerplate.** Group selected postings by `company_id`. For each company with ≥3 selected postings, pull all of that company's latest-snapshot descriptions via `ListLatestDescriptionsByCompany`, run them through the `boilerplate.Strip` helper, and substitute the cleaned text for `description_text` in the agent prompt for postings in that company. Companies with <3 postings in the batch pass through unchanged. The helper is invoked via a small Go entry point (a `cmd/strip-boilerplate` thin CLI or `go run` against a single-file driver — implementer's choice) since the skill orchestrator is shell+psql, not Go-linked.

State explicitly in the skill: this preprocessor never writes to the DB. The canonical `posting_snapshots.description_text` row is unchanged.

The detection corpus is "all latest descriptions for the company," not just the postings selected for this batch. Selection LIMITs and ILIKE filters narrow what gets enriched, but boilerplate detection wants the largest sample available for that company.

### Tests

Unit tests in `internal/enrich/boilerplate/strip_test.go`. Black-box (`package boilerplate_test`) where it doesn't force exporting internals.

Cases:

- **Abundant samples, clear boilerplate.** N=5 inputs sharing a 4-paragraph company blurb plus distinct role bodies. Assert: blurb absent from every output; role bodies preserved verbatim.
- **Below bootstrap threshold.** N=2 with obvious shared text. Assert: outputs equal inputs.
- **Diverging samples.** N=8 with no shared paragraph meeting both filters. Assert: outputs equal inputs.
- **Entire description is boilerplate.** One input in the set is exactly the shared blurb with nothing else. Assert: that input is returned unchanged (empty-residue safety); others have the blurb stripped.
- **Multiple disjoint boilerplate blocks.** Inputs share two separate qualifying blocks (e.g., a company blurb at the top and an EEO footer). Assert: both stripped.
- **Order preservation.** Outputs match inputs index-for-index.

DB integration test in `internal/db/` (build tag `//go:build integration`) for `ListLatestDescriptionsByCompany`: insert two postings for one company with multiple snapshots each (one snapshot has NULL `description_text`); assert the query returns the latest non-null per posting, skips the NULL-tail posting if its latest is NULL.

## Sequencing

**Phase 1 (concurrent):** Helper package + tests (`internal/enrich/boilerplate`); sqlc query addition + integration test (`internal/db/queries/fetcher.sql`). Independent files, independent tests.
**Phase 2 (sequential):** Skill edit (`.claude/skills/batch-enrich/SKILL.md`) + CLI/`go run` driver. Consumes the helper signature and query name from Phase 1.

## Open questions

- Should the CLI/driver be a real `cmd/strip-boilerplate` binary or an inline `go run ./internal/enrich/boilerplate/cmd` invocation from the skill? Real binary is cleaner; `go run` avoids a build step. Implementer's call.
- Is paragraph anchoring tight enough, or do we want sentence-level anchoring as a fallback for descriptions that are one giant paragraph? Defer until a real failure case appears — the 5-posting Cognition sample showed paragraph-anchored boilerplate, which is the v1 target.
- `minMatchLen=200` and `minPrevalence=0.6` are guesses calibrated to the Cognition observation. Implementer should sanity-check on at least one other watchlist company's snapshots before locking defaults.
