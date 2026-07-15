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
      another); each matched company row states which signal(s) produced it.
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

Add a new migration enabling `pg_trgm`
(`CREATE EXTENSION IF NOT EXISTS pg_trgm;`), mirroring how migration 000001
enables `vector`. Add a new sqlc query in `onboard.sql`,
`FindCompaniesByNameSimilarity`, following the existing
`FindCompaniesByNormalizedNames` shape: batched input array, `WITH
ORDINALITY` for input ordering, `has_recent_snapshot` computed the same way.
Use Postgres's `similarity()` function (from `pg_trgm`) between each
candidate's normalized name and each company's normalized name; keep rows at
or above a threshold constant (mirror `dedupDefaultRecencyDays`'s pattern —
a named constant near the top of `dedup_candidates.go`, not a magic number
inline). Order matches by similarity descending.

In `runDedupCandidates`, call this query only for candidates that resolved
to verdict `new` after token and exact-name matching — running fuzzy
matching against every candidate regardless of exact-match outcome would
double-report companies already caught by the stronger signal. A company
already matched by exact name for a given candidate must not also appear as
a `fuzzy_name` match for that same candidate.

Cap the `fuzzy_name` rows surfaced per candidate at 3, keeping the highest
similarity scores. Company-name similarity has a noisy long tail once you're
near the threshold — a match that far down the ranking rarely changes an
agent's or reviewer's decision, and an uncapped list risks flooding a
batch response when a candidate's name is generic enough to score against
many rows. Preserve the true match count (pre-cap) separately from the
capped list — see Task 3.

### Task 2: Accept `careers_url` and add domain matching

Add `careers_url` (optional, string) to `dedupCandidateInput` and its MCP
tool schema entry in `main.go`. Validate with the existing
`atsdetect.ValidateURL` helper (already used by `add_company.go` for the
same field name on that tool) — an invalid URL is not a candidate-level
error; treat it as absent and continue with token/name matching only.

Add a new sqlc query, `FindCompaniesByCareersURLHost`, comparing an
extracted host from the candidate-supplied URL against an extracted host
from `companies.careers_page_url`. Mirror
`FindCompaniesByNormalizedNames`'s pattern of repeating the same extraction
expression verbatim in both the candidate subquery and the companies
subquery — that's the existing precedent for this kind of paired
normalization in this file, rather than introducing a SQL helper function.
Companies with a `NULL` `careers_page_url` are excluded naturally.

### Task 3: Merge signals into the result shape

Extend `dedupMatchedCompany` with a `match_kind` field (`"token"`,
`"name_only"`, `"domain"`, or `"fuzzy_name"`) and a nullable
`similarity_score` field (populated only for `fuzzy_name` matches). A
candidate's `matches` list is the union of every signal's matches,
deduplicated by company `id` — if a company matches on more than one signal,
keep the strongest per the priority order `token > domain > name_only >
fuzzy_name` and note this in the boundary inventory below. `domain` outranks
`name_only`: a URL match is stronger evidence than a name match, since
company names collide far more often than hostnames do. The result-level
`match_kind` / `reason` reflect the strongest signal present across all
matches for that candidate. Verdict stays `stale` whenever any non-token
signal produced a match — only a `token` match can produce `duplicate`.

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
| Match signal | `MatchKind` (on `dedupMatchedCompany`) | `"match_kind"` | n/a (computed in Go) |
| Fuzzy similarity | `SimilarityScore *float64` | `"similarity_score"` | `similarity(...)` (pg_trgm) |
| New match_kind values | — | `"domain"`, `"fuzzy_name"` | — |
| New reason values | — | `"matched_by_domain"`, `"matched_by_fuzzy_name"` | — |

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
