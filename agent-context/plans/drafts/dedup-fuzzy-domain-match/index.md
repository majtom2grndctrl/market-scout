# Dedup Candidates: Fuzzy Name and Domain Matching

## Goal

`dedup_candidates` only catches duplicates on exact `(ats, board_token)` or
an exact-normalized-name equality. A company known by a slightly different
name ("Helion" vs "Helion Energy") surfaces as `new`, sending an agent into
unnecessary browser investigation. Add two additional, weaker-confidence
match signals — trigram name similarity and careers-page domain — so more
true duplicates surface for review before browser work starts, without
weakening the existing skip rule (only `(ats, board_token)` may ever produce
verdict `duplicate`).

## Scope

### In scope

- Enable `pg_trgm` and add a trigram similarity match against
  `companies.name` for candidates that have no exact token or exact-name
  match.
- Accept an optional `careers_url` per candidate and match it against the
  host of existing companies' `careers_page_url`.
- Extend `dedupCandidateResult` / `dedupMatchedCompany` to carry enough
  information to distinguish which signal(s) produced each match.
- Document the new signals and their priority in `agent-context/lib/watchlist.md`.

### Out of scope

- A GIN trigram index on `companies.name`. Current company count is small
  enough that a sequential scan is fine; add an index if row count growth
  makes it a measured problem.
- Exposing the similarity threshold as a request parameter. Starts as a Go
  constant; revisit if a real tuning need shows up.
- Any change to the `(ats, board_token)` skip rule or to when `add_company`
  is safe to call. Fuzzy and domain matches never produce verdict
  `duplicate` — they route to `stale`, same as today's name-only matches.
- Matching against a company `homepage_url` — no such column exists;
  `careers_page_url` is the only URL field in the schema, and is what this
  spec compares against.
- Schema change to add a `domain` column. Domain is derived at query time
  from `careers_page_url`, same as `name` is normalized at query time today.

## Acceptance criteria

- [ ] `pg_trgm` is enabled via a new numbered migration.
- [ ] A candidate whose normalized name has no exact match, but whose name
      scores at or above the similarity threshold against an existing
      company's name, is returned with a match carrying `match_kind:
      "fuzzy_name"` and a similarity score.
- [ ] A candidate with an exact-normalized-name match to a company never
      also receives a `fuzzy_name` match for that same company — exact match
      wins for that company.
- [ ] `dedup_candidates` accepts an optional `careers_url` field per
      candidate. When supplied and its host matches the host of an existing
      company's `careers_page_url`, that company is returned with a match
      carrying `match_kind: "domain"`.
- [ ] An invalid or unparsable `careers_url` on a candidate does not fail
      the candidate or the batch — it is treated as if no `careers_url` was
      supplied, and token/name matching still runs.
- [ ] A single candidate can surface matches from more than one signal in
      one call (e.g. a domain match to one company and a fuzzy-name match to
      another); each matched company row states the strongest signal that
      produced it.
- [ ] When one company matches a candidate on more than one signal, the
      merged row's `match_kind` resolves to the highest-priority signal per
      `token > domain > name_only > fuzzy_name`.
- [ ] `reason` is set to `matched_by_domain` for a `domain` match and
      `matched_by_fuzzy_name` for a `fuzzy_name` match, alongside the
      existing `matched_by_token_*` and `matched_by_name_only` values.
- [ ] Verdict for every match produced by the `domain` or `fuzzy_name`
      signal is `stale`, never `duplicate`.
- [ ] `fuzzy_name` matches surfaced in a candidate's `matches` list are
      capped at 3, ranked by similarity descending. `match_count` reports
      the true number of fuzzy matches found at or above the threshold,
      even when it exceeds 3 — the cap trims what's returned, not what's
      counted.
- [ ] Existing behavior is unchanged for candidates that already resolve via
      token match or exact-name match — no regression in `match_kind:
      "token"` or `match_kind: "name_only"` results.
- [ ] `agent-context/lib/watchlist.md` §Dedup documents the new signals,
      the similarity threshold, and how they rank against the existing
      token/exact-name signals.

## Tasks

### Task 1: Enable `pg_trgm` and add fuzzy-name matching

- Add a new migration enabling `pg_trgm`
  (`CREATE EXTENSION IF NOT EXISTS pg_trgm;`). Mirrors how migration 000001
  enables `vector`.
- Add a new sqlc query in `onboard.sql`, `FindCompaniesByNameSimilarity`,
  matching `FindCompaniesByNormalizedNames`'s shape: batched input array,
  `WITH ORDINALITY` for input ordering, `has_recent_snapshot` computed via
  the same `EXISTS` subquery.
- Compare each candidate's normalized name against each company's
  normalized name with `pg_trgm`'s `similarity()` function; keep only rows
  at or above a threshold.
- Hold the threshold in a named constant next to the existing
  `dedupDefaultRecencyDays` / `dedupMaxCandidates` constants
  (`dedup_candidates.go` lines ~17-37) — a magic number buried in a query is
  invisible to the next person tuning it.
- Order results by similarity descending, so the cap (below) keeps the
  strongest matches.
- Add `FindCompaniesByNameSimilarity` to the `dedupSource` interface
  (`dedup_candidates.go:79-82`), implement it on `poolDedupSource`, and add
  a fake to `fakeDedupSource` (`dedup_candidates_test.go`) —
  `runDedupCandidates` only reaches the DB through `dedupSource`; calling
  the sqlc function directly bypasses that seam and the existing test
  doubles.
- Call this query only for candidates that resolved to verdict `new` after
  token and exact-name matching.
- Cap the `fuzzy_name` rows surfaced per candidate at 3, keeping the highest
  scores. Similarity has a noisy long tail near the threshold, and an
  uncapped list risks flooding a batch response when a candidate's name is
  generic enough to score against many rows.
- Preserve the true fuzzy-match count separately from the capped list for
  `match_count` — see Task 3.

Do not:
- Run fuzzy matching against a candidate that already resolved via token or
  exact-name match. The stronger signal already caught it; re-running fuzzy
  would double-report the same company.
- Call sqlc-generated query functions directly from `runDedupCandidates`.
  Go through `dedupSource`.

### Task 2: Accept `careers_url` and add domain matching

- Add `careers_url` (optional, string) to `dedupCandidateInput` and its MCP
  tool schema entry in `main.go`.
- Validate with the existing `atsdetect.ValidateURL` helper
  (`apps/tools/internal/atsdetect/detect.go`) — the same validator
  `add_company.go` uses for its `careers_page_url` field.
- Treat an invalid or unparsable URL as if `careers_url` were absent for
  that candidate; do not fail the candidate or the batch.
- Add a new sqlc query, `FindCompaniesByCareersURLHost`, comparing an
  extracted host from the candidate-supplied URL against an extracted host
  from `companies.careers_page_url`. Mirror `FindCompaniesByNormalizedNames`'s
  pattern of repeating the same extraction expression verbatim in both the
  candidate subquery and the companies subquery — the existing precedent for
  this kind of paired normalization in this file, rather than a new SQL
  helper function.
- Companies with a `NULL` `careers_page_url` are excluded naturally by this
  join.
- Add `FindCompaniesByCareersURLHost` to the `dedupSource` interface,
  `poolDedupSource`, and `fakeDedupSource`, same as Task 1's fuzzy-name
  query.

Do not:
- Surface a validation error for a malformed `careers_url` as a
  candidate-level failure. It is a missing optional signal, not an error.

### Task 3: Merge signals into the result shape

- Add a `match_kind` field (`"token"`, `"name_only"`, `"domain"`, or
  `"fuzzy_name"`) and a nullable `similarity_score` field to
  `dedupMatchedCompany`, populated only for `fuzzy_name` matches. This is a
  new, per-company field — distinct from the existing result-level
  `MatchKind` on `dedupCandidateResult` (`dedup_candidates.go:61`), which is
  reused below to carry the strongest signal across the whole candidate.
- Build each candidate's `matches` list as the union of every signal's
  matches, deduplicated by company `id`.
- When one company matches on more than one signal, keep the strongest per
  the priority order `token > domain > name_only > fuzzy_name`. `domain`
  outranks `name_only`: a URL match is stronger evidence than a name match,
  since company names collide far more often than hostnames do.
- Set the result-level `match_kind` / `reason` (on `dedupCandidateResult`)
  to the strongest signal present across all matches for that candidate.
- Verdict stays `stale` whenever any non-token signal produced a match —
  only a `token` match can produce `duplicate`.

`match_count` is the true number of matches found across all signals before
the `fuzzy_name` cap is applied (see Task 1) — it can exceed
`len(matches)` when a candidate's fuzzy matches were trimmed to 3. This
keeps the cap from reading as silent under-reporting: the count always
reflects what was found, only the returned rows are trimmed.

### Task 4: Document in `watchlist.md`

Add the two new signals to §Dedup: what they compare, the similarity
threshold value, the fuzzy-match cap (top 3 by score, true count still
reported), and that both new signals route to `stale-needs-merge` review,
never a silent duplicate. State the signal priority: token > domain >
name_only > fuzzy_name.

## Sequencing

**Phase 1 (sequential):** Task 1 — establishes the migration and the fuzzy
query pattern other tasks don't depend on, but should land first since it
touches the schema.
**Phase 2 (concurrent):** Task 2 — independent of Task 1; different query,
different input field.
**Phase 3 (sequential):** Task 3 — consumes the query results from Tasks 1
and 2 to build the merged response shape.
**Phase 4 (concurrent):** Task 4 — documentation, independent of code once
the signal names and priority are settled (they're settled by Task 3's
design, not its implementation).

## Boundary inventory

| Name | Go struct field | JSON key | SQL column / expression |
|---|---|---|---|
| Candidate careers URL | `CareersURL` | `"careers_url"` | n/a (query param) |
| Per-company match signal (new) | `MatchKind` (on `dedupMatchedCompany`) | `"match_kind"` | n/a (computed in Go) |
| Result-level match signal (existing, reused) | `MatchKind` (on `dedupCandidateResult`, `dedup_candidates.go:61`) | `"match_kind"` | n/a — not new, carries the strongest signal |
| Fuzzy similarity | `SimilarityScore *float64` | `"similarity_score"` | `similarity(...)` (pg_trgm) |
| New match_kind values | — | `"domain"`, `"fuzzy_name"` | — |
| New reason values | — | `"matched_by_domain"`, `"matched_by_fuzzy_name"` | — |

`similarity_score` follows the existing `dedupMatchedCompany` field
precedent (`Industry *string`, `CareersPageURL *string`, neither tagged
`omitempty`): a nil pointer serializes as JSON `null`, not an omitted key.

## Rough sketch

`dedup_candidates.go`: add `dedupFuzzyNameSimilarityThreshold` constant near
existing `dedupDefaultRecencyDays` / `dedupMaxCandidates` constants (line
~17-37). Host extraction expression (candidate for both Task 1's name-norm
precedent and Task 2's new expression):

```sql
lower(regexp_replace(url, '^https?://(www\.)?([^/]+).*$', '\2'))
```

Threshold starting point: 0.4. `pg_trgm`'s own default
(`pg_trgm.similarity_threshold`) is 0.3, but company names are often short,
where trigram similarity on short strings runs noisier — start higher and
revisit after real usage.
