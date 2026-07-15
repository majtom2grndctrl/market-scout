# Research notes: dedup_candidates fuzzy + domain matching

Findings from reading current source, kept out of the spec per style-guide
§Documentation Lifecycle ("research notes stay out of the spec").

## Current `dedup_candidates` implementation

`apps/tools/cmd/mcp/dedup_candidates.go` (294 lines).

Input per candidate (`dedupCandidateInput`): `name` (required), `ats`
(optional), `board_token` (optional). No URL field today.

Algorithm (`runDedupCandidates`, lines 163-272):
1. Per candidate, if `ats` + `board_token` both supplied, call
   `FindCompanyDedupStatus` (exact `(ats, board_token)` match against the
   `uq_companies_ats_board_token` unique constraint). Match found →
   `match_kind: token`, verdict `duplicate` (recent snapshot) or `stale` (no
   recent snapshot). This short-circuits — name lookup is skipped.
2. Otherwise, candidate is queued into a batched call to
   `FindCompaniesByNormalizedNames`. Match found → `match_kind: name_only`,
   verdict always `stale` (never `duplicate` — names collide across real
   companies). No match → verdict `new`.

Response shape (`dedupCandidateResult` / `dedupMatchedCompany`, lines 50-77):
top-level `verdict`, `match_kind`, `reason`, `match_count`, `matched` (first
row), `matches` (all colliding rows). Each matched company carries `id`,
`name`, `ats`, `board_token`, `industry`, `careers_page_url`,
`has_recent_snapshot`. No per-row `match_kind` today — one `match_kind` for
the whole candidate result.

## sqlc queries (`apps/tools/internal/db/queries/onboard.sql`)

`FindCompanyDedupStatus` — exact `WHERE c.ats = ... AND c.board_token = ...`.

`FindCompaniesByNormalizedNames` — normalizes both the candidate name array
and `companies.name` with the identical expression
`lower(regexp_replace(name, '[^[:alnum:]]', '', 'g'))`, then joins on exact
equality. The normalize expression is duplicated verbatim in both the
candidate CTE and the companies CTE — this is existing precedent for
duplicating a small normalization expression across two subqueries in one
statement, rather than factoring it into a SQL function.

Both queries also compute `has_recent_snapshot` via the same
`EXISTS (... posting_snapshots ... fetched_at >= now() - make_interval(...))`
subquery, duplicated across both files.

## Schema

`companies` table (`apps/tools/internal/db/migrations/000001_initial_schema.up.sql`):
`id, name, ats (NOT NULL as of migration 000002), board_token (NOT NULL),
created_at, employee_count_range, founded_year, funding_stage,
total_funding_usd, industry, company_type, careers_page_url, enriched_at`.
Unique constraint `(ats, board_token)`.

No `domain` or `homepage_url` column anywhere in the schema (checked all 13
migrations). The only URL-shaped field is `careers_page_url` (free text,
never parsed or compared today — selected and returned as a display field
only).

`pg_trgm` is not enabled. Only extension installed is `vector`
(`CREATE EXTENSION IF NOT EXISTS vector;`, migration 000001 line 5).

## `add_company` (for field-shape precedent)

`apps/tools/cmd/mcp/add_company.go`. Accepts `name`, `ats`, `board_token`,
`industry`, `careers_page_url`, `probe`. Validates `careers_page_url` via
`atsdetect.ValidateURL` when present (line ~304-335 `validateAddCompany`).
Writes through `mcp.add_company` SECURITY DEFINER function — plain insert,
`ON CONFLICT (ats, board_token) DO NOTHING`, never updates existing rows.

`atsdetect.ValidateURL` is the existing URL-validation helper to reuse for a
new `careers_url` input field on `dedup_candidates`, rather than writing a
second validator.

## Trust tier placement (`project.md` §Evidence trust tiers)

`dedup_candidates` verdicts are Deterministic tier: computed from
agent-supplied evidence (name, and now optionally a URL), reproducible, but
no more trustworthy than the input. Fuzzy-name and domain matches stay in
this tier — same as the existing exact-name match. They are stronger or
weaker *confidence* signals within the tier, not a different tier. Neither
new signal should ever produce verdict `duplicate` — only `(ats,
board_token)` is tool-attested ground truth per project.md.
