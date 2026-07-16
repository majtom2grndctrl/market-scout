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
- [ ] Domain matching runs regardless of the exact-name outcome: a candidate
      whose name exact-matches one company can still receive a separate
      `domain` match to a different company in the same call.
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
      the true number of distinct companies matched across all signals for
      that candidate, deduplicated by company id, before the fuzzy cap
      trims the returned list — the cap trims what's returned, not what's
      counted.
- [ ] Existing behavior is unchanged for candidates that already resolve via
      token match or exact-name match — no regression in `match_kind:
      "token"` or `match_kind: "name_only"` results.
- [ ] The trigram similarity query and the host-extraction query each have
      an `integration`-tagged test against a real Postgres, verifying the
      actual scoring/ordering and host-matching behavior. The existing
      `dedup_candidates_test.go` suite only exercises orchestration against
      a faked `dedupSource` and never proves the SQL itself is correct.
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
  normalized name with `pg_trgm`'s `similarity()` function. Cast the
  result — `similarity(...)::float8 AS score` — so sqlc generates a Go
  `float64`. Uncast, `similarity()` returns Postgres `real` and sqlc
  generates `float32` instead, which won't assign to the `*float64` field
  below.
- Keep only rows scoring at or above `0.4`. Hold this value in a named
  constant, `dedupFuzzyNameSimilarityThreshold`, next to the existing
  `dedupDefaultRecencyDays` / `dedupMaxCandidates` constants
  (`dedup_candidates.go` lines ~17-37) — a magic number buried in a query is
  invisible to the next person tuning it. `pg_trgm`'s own default
  (`pg_trgm.similarity_threshold`) is 0.3; start higher because company
  names are often short, where trigram similarity runs noisier.
- Order results by similarity descending, so the cap (below) keeps the
  strongest matches.
- Add a `MatchKind` field (`"token"`, `"name_only"`, `"domain"`, or
  `"fuzzy_name"`) and a nullable `SimilarityScore *float64` field to
  `dedupMatchedCompany` now, in this task — Task 2's domain query and Task
  3's merge both need these fields to already exist when their phases run.
  Populate `SimilarityScore` from this query's cast score; leave
  `MatchKind` unset here — Task 3 stamps it centrally during the merge.
- Add `FindByNameSimilarity` to the `dedupSource` interface
  (`dedup_candidates.go:79-82`), implement it on `poolDedupSource`, and add
  a fake to `fakeDedupSource` (`dedup_candidates_test.go`) —
  `runDedupCandidates` only reaches the DB through `dedupSource`; calling
  the sqlc function directly bypasses that seam and the existing test
  doubles.
- Add an `integration`-tagged test exercising the real `similarity()` query
  against Postgres (testing-guide.md §1 pattern). The unit tests in
  `dedup_candidates_test.go` only exercise orchestration against a faked
  `dedupSource` and never prove the SQL itself scores or orders correctly.
- This query is the last one called per candidate, after token, exact-name,
  and domain matching have all had a chance to match — Task 3 pins the
  exact call order. Fuzzy is the last-resort signal; a candidate already
  flagged for review by a stronger signal doesn't need a noisier one spent
  on it too.
- Cap the `fuzzy_name` rows surfaced per candidate at 3, keeping the highest
  scores, applied in Go after the true count is recorded — not a SQL
  `LIMIT`, which would destroy the true count `match_count` needs (see
  Task 3). Similarity has a noisy long tail near the threshold, and an
  uncapped list risks flooding a batch response when a candidate's name is
  generic enough to score against many rows.
- Preserve the true fuzzy-match count separately from the capped list for
  `match_count` — see Task 3.

Do not:
- Run fuzzy matching against a candidate that already resolved via token,
  domain, or exact-name match. A stronger signal already caught it;
  re-running fuzzy would only add noise.
- Call sqlc-generated query functions directly from `runDedupCandidates`.
  Go through `dedupSource`.

### Task 2: Accept `careers_url` and add domain matching

- Add `careers_url` (optional, string) to `dedupCandidateInput`.
- Add `careers_url` to the MCP tool schema in `main.go` — nested inside the
  `candidates` array's `mcp.Items(map[string]any{"properties": {...}})`
  per-item property map (`main.go` ~205-213), alongside `name`/`ats`/
  `board_token`. This is a different location from `detect_ats`'s
  `careers_url` field (`main.go:222`), which is a top-level
  `mcp.WithString` tool option, not a per-item array property — don't
  copy that pattern; it would make `careers_url` a batch-level field
  instead of a per-candidate one.
- Validate with the existing `atsdetect.ValidateURL` helper
  (`apps/tools/internal/atsdetect/detect.go`) — the same validator
  `add_company.go` uses for its `careers_page_url` field.
- Treat an invalid or unparsable URL as if `careers_url` were absent for
  that candidate; do not fail the candidate or the batch. This validation
  runs inline in `runDedupCandidates`'s per-candidate loop, where Task 3
  collects candidates for the batched domain lookup.
- Add a new sqlc query, `FindCompaniesByCareersURLHost`, comparing an
  extracted host from the candidate-supplied URL against an extracted host
  from `companies.careers_page_url`, using this expression on both sides:
  `lower(regexp_replace(url, '^https?://(www\.)?([^/]+).*$', '\2'))`.
  Mirror `FindCompaniesByNormalizedNames`'s pattern of repeating the same
  extraction expression verbatim in both the candidate subquery and the
  companies subquery — the existing precedent for this kind of paired
  normalization in this file, rather than a new SQL helper function.
- Companies with a `NULL` `careers_page_url` are excluded naturally by this
  join.
- Add `FindByCareersURLHost` to the `dedupSource` interface,
  `poolDedupSource`, and `fakeDedupSource`, same as Task 1's fuzzy-name
  query.
- Add an `integration`-tagged test exercising the real host-extraction
  query against Postgres, same as Task 1. The extraction regex is
  load-bearing — it decides whether `www.acme.com` matches `acme.com` — and
  is never exercised by the faked unit tests.
- Run this query for every candidate with a `careers_url`, regardless of
  the token or exact-name outcome. `watchlist.md` §Dedup already treats the
  board/careers URL as the strongest disambiguation signal after token —
  gating it behind a name-match miss would mean it never fires for a
  candidate whose name happens to collide with an unrelated company (the
  exact case that signal exists to catch).

Do not:
- Surface a validation error for a malformed `careers_url` as a
  candidate-level failure. It is a missing optional signal, not an error.
- Gate domain matching on the exact-name result. Unlike fuzzy matching,
  domain isn't a last-resort signal — it needs to run unconditionally to
  correct a wrong name match, not just catch a missed one.

### Task 3: Merge signals into the result shape

- `runDedupCandidates` is two-phase today: a per-candidate loop resolves
  token matches and collects the rest for one batched `FindByNames` call
  after the loop. Extend this to four phases, in order:
  1. Per-candidate loop (existing): resolve `token` matches by
     short-circuiting as today. For every candidate without a token match,
     collect it into the existing name-lookup batch — and, if it has a
     validated `careers_url` (Task 2), into the domain-lookup batch too.
  2. Batched lookups: run the existing `FindByNames` and Task 2's
     `FindByCareersURLHost` unconditionally, for every candidate that
     reached this phase — domain never waits on the name-batch result.
  3. Compute the still-unresolved set: candidates with no result from
     either batch in step 2.
  4. Batched fallback: run Task 1's `FindByNameSimilarity` only against the
     step-3 set.
- `dedupMatchedCompany` already has `MatchKind` and `SimilarityScore` —
  Task 1 added both fields. Stamp `MatchKind` on every row this merge
  handles, including the **existing** exact-name-match rows from step 2's
  `FindByNames` call: those rows carry no per-row `match_kind` today
  (`runDedupCandidates` only sets the result-level field), so this task
  must add `"name_only"` tagging to that existing path too, not just to the
  two new signals.
- Build each candidate's `matches` list as the union of every signal's
  matches from steps 2 and 4, deduplicated by company `id`.
- When one company matches on more than one signal, keep the strongest per
  the priority order `token > domain > name_only > fuzzy_name`. `domain`
  outranks `name_only`: a URL match is stronger evidence than a name match,
  since company names collide far more often than hostnames do.
- Set the result-level `match_kind` / `reason` (on `dedupCandidateResult`,
  the existing field at `dedup_candidates.go:61` — distinct from the new
  per-company field on `dedupMatchedCompany`) to the strongest signal
  present across all matches for that candidate.
- Verdict stays `stale` whenever any non-token signal produced a match —
  only a `token` match can produce `duplicate`.

`match_count` is the number of distinct companies matched across all
signals for that candidate, counted after the cross-signal dedup-by-id
above but before the `fuzzy_name` cap (see Task 1) trims the returned list
— it can exceed `len(matches)` when a candidate's fuzzy matches were
trimmed to 3. Domain and name_only are never capped, so `match_count` and
`len(matches)` only diverge because of the fuzzy trim. This keeps the cap
from reading as silent under-reporting: the count always reflects what was
found, only the returned rows are trimmed.

### Task 4: Document in `watchlist.md`

Add the two new signals to §Dedup: what they compare, the similarity
threshold value (`0.4`), the fuzzy-match cap (top 3 by score, true count
still reported), and that both new signals route to `stale-needs-merge`
review, never a silent duplicate. State the signal priority: token >
domain > name_only > fuzzy_name.

## Sequencing

**Phase 1 (sequential):** Task 1 — establishes the migration, the fuzzy
query pattern, and the `MatchKind` / `SimilarityScore` fields on
`dedupMatchedCompany` that Task 3 depends on. Lands first since it touches
the schema and the shared struct.
**Phase 2 (concurrent):** Task 2 — independent of Task 1's query and
fields; adds its own query and input field.
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

The `*float64` type depends on `FindCompaniesByNameSimilarity` casting the
SQL `similarity()` output to `float8` (Task 1) — uncast, sqlc infers
`float32` for Postgres `real`, which won't assign to a `*float64` field.

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
