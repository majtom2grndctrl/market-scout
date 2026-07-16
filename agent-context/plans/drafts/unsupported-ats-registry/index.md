# Unsupported ATS Registry

## Goal

Persist companies with a known-unsupported ATS, or no discoverable careers page, so `dedup_candidates` can warn agents off re-investigating them in the browser. Surfaced as informational metadata only — never a hard skip — matching the Deterministic trust tier the rest of `dedup_candidates` already holds itself to.

## Scope

### In scope
- New table `unsupported_companies` and its write path: `mcp.record_unsupported_company` (SECURITY DEFINER function) plus a `record_unsupported_ats` MCP action tool, mirroring `add_company`'s write-gate pattern.
- `dedup_candidates` gains a `known_unsupported` field per result, populated from the new table.
- `discovery-run` skill calls the new tool on `unsupported-ats` and `no-careers` outcomes instead of just reporting and dropping them.
- `watchlist.md` documents the registry.

### Out of scope
- No change to existing `verdict` / `match_kind` / `matches` machinery — `known_unsupported` is a separate, independent signal.
- No automatic re-investigation or scheduling. Staleness is surfaced as data; an agent decides whether to act on it.
- No retroactive backfill of past discovery-run sessions' unsupported findings. The registry starts empty and grows going forward.
- `apps/tools/cmd/onboard` (the batch/sidecar research path) is untouched. It already has its own tool-verified `no-careers` status (an HTTP probe) with a different lifecycle; unifying the two is a separate decision, not made here.

## Acceptance criteria
- [ ] Migration creates `unsupported_companies` with a functional unique index on the normalized name expression `FindCompaniesByNormalizedNames` already uses.
- [ ] `mcp.record_unsupported_company` inserts a new row on first sight of a normalized name, and on a repeat sighting updates `last_checked_at` to now and overwrites `url` / `detected_platform` / `reason` only where the caller supplied a non-null value.
- [ ] `record_unsupported_ats` validates: `name` required; `reason` is exactly `unsupported_ats` or `no_careers`; `url` required when `reason` is `unsupported_ats`, optional when `no_careers`; a syntactically invalid `url` is a validation error, not a silently dropped field.
- [ ] `dedup_candidates` returns `known_unsupported: null` by default, populated for any candidate that did not resolve via token match and whose normalized name or URL host matches a row in `unsupported_companies`.
- [ ] A populated `known_unsupported` includes `reason`, `detected_platform` (nullable), `first_seen_at`, `last_checked_at`, and `stale` (`true` when `last_checked_at` is more than 90 days old).
- [ ] `known_unsupported` never changes `verdict`, `match_kind`, `matches`, or `match_count` for the same result — a candidate can be `verdict: "new"` and still carry a populated `known_unsupported`.
- [ ] When a candidate's lookup matches `unsupported_companies` by both URL host and name, the host match wins — mirrors the existing `domain > name_only` priority for company matches.
- [ ] `record_unsupported_ats` and the two new dedup lookup queries each have an `integration`-tagged test against a live Postgres instance.
- [ ] `discovery-run` calls `record_unsupported_ats` when `detect_ats` returns `unsupported-ats`, and when browser investigation concludes `no-careers`, instead of only reporting the status and dropping it.
- [ ] `watchlist.md` documents what triggers a registry write, the 90-day revisit threshold, and that `known_unsupported` informs but never gates an agent's decision to re-investigate.

## Tasks

### Task 1: Unsupported companies table and write path

- Add a migration creating `unsupported_companies` (see Rough sketch for exact DDL) with a functional unique index on `lower(regexp_replace(name, '[^[:alnum:]]', '', 'g'))` — the same expression `FindCompaniesByNormalizedNames` uses. Reusing it, rather than inventing a second normalization rule, keeps write-time and read-time identity checks consistent.
- Add `mcp.record_unsupported_company` as a SECURITY DEFINER function in a companion migration, hardened the same way as `mcp.add_company`: owned by the migration-running role, `SET search_path = pg_catalog`, `REVOKE ALL ... FROM PUBLIC`, then an explicit grant added to `internal/db/setup/action_role.sql`. On conflict against the unique index, update `last_checked_at = now()` and take the incoming `url` / `detected_platform` / `reason` only where non-null.
- This function returns multiple columns via `RETURNS TABLE`, which sqlc's offline parser cannot expand without a live database (`developer-guide.md` §5.7) — the same limitation `add_company` already works around. Call it with a fixed, fully-parameterized `QueryRowContext`, mirroring `poolExecutor` / `addCompanyExecutor` in `add_company.go`, not a sqlc query file.
- Add the MCP action tool `record_unsupported_ats` in a new `apps/tools/cmd/mcp/record_unsupported_ats.go`, registered in `main.go` bound to `pools.action` — it writes through an approved function, same reasoning as `add_company`'s pool binding. Request fields: `name` (required), `url` (required when `reason` is `unsupported_ats`, optional otherwise; validate with `atsdetect.ValidateURL` when present), `detected_platform` (optional free text), `reason` (required, one of `unsupported_ats` / `no_careers`). Mirror `add_company`'s envelope shape (`Ok`, `Errors`) and its injectable-executor test seam.

Do not:
- Route this write through the read-only query gateway, or generate it as a sqlc function.
- Store this data in `companies` or reuse `mcp.add_company` — it is a separate registry with a separate lifecycle (see Scope).

### Task 2: Surface known_unsupported in dedup_candidates

- Add two `dedupSource` methods mirroring the shape of `FindByNames` / `FindByCareersURLHost`: `FindUnsupportedByNames(ctx, names []string) (map[int]dedupUnsupportedRecord, error)` and `FindUnsupportedByURLHost(ctx, urls []string) (map[int]dedupUnsupportedRecord, error)`. Each is backed by a new sqlc query in `onboard.sql` — a plain `:many` SELECT, not a `RETURNS TABLE` function, so sqlc generates it without issue. Reuse the exact normalization and host-extraction expressions from `FindCompaniesByNormalizedNames` and `FindCompaniesByCareersURLHost` so identity matches the same way on both sides.
- In `runDedupCandidates`, call both against the `nameLookupNames` / `domainLookupURLs` batches already gathered for company matching — no new input-gathering logic needed. Run this only for candidates that did not resolve via token match; a token match means the company is already tracked, so it cannot also be in `unsupported_companies`.
- When both the URL-host and name lookups hit for the same candidate, the URL-host result wins — mirrors the existing `domain > name_only` priority for company matches.
- Add `KnownUnsupported *dedupUnsupportedRecord` (`json:"known_unsupported"`) to `dedupCandidateResult`, and a new `dedupUnsupportedRecord` struct (see Boundary inventory for field/JSON pins). Compute `Stale` in Go against a new constant `dedupUnsupportedRevisitDays = 90`, placed near `dedupFuzzyNameSimilarityThreshold`.

Do not:
- Let a populated `known_unsupported` change `Verdict`, `MatchKind`, `Matches`, or `MatchCount` — set it independently of the existing match-merging logic in `mergeDedupMatches`.
- Compute the 90-day staleness window in SQL. It is a fixed constant, unlike the caller-configurable `recency_days` window the rest of the tool uses.

### Task 3: Wire discovery-run to record unsupported outcomes

- In `.claude/skills/discovery-run/SKILL.md`, after `detect_ats` returns `unsupported-ats`, call `record_unsupported_ats` with `reason: "unsupported_ats"` and the URL evidence that produced the result, before recording the candidate's final status.
- When browser investigation finds no usable careers page or board at all (today's `no-careers` status), call `record_unsupported_ats` with `reason: "no_careers"`, the homepage URL if one was found (otherwise omit `url`), and no `detected_platform`.
- Add a line to the Safety rules section: call `record_unsupported_ats` for both outcomes so later runs don't repeat the same browser investigation.

Do not:
- Call `record_unsupported_ats` for `invalid-token` or `ambiguous` outcomes — those mean a supported ATS was found but needs a different token or disambiguation, not that the ATS itself is unsupported.

### Task 4: Document the registry

- In `watchlist.md` §Dedup, document `known_unsupported`: what it is, that it is informational only, and that it never changes verdict or match_kind.
- Add a new subsection documenting the registry itself: what triggers a write (`unsupported-ats` from `detect_ats`, `no-careers` from browser investigation), the fields captured, and the 90-day revisit threshold — after which `stale: true` signals the agent that a re-check may be worthwhile, not that one is required.

## Sequencing

**Phase 1 (sequential):** Task 1 — defines the table, the write function, and the tool the other tasks assume exist.
**Phase 2 (concurrent):** Task 2, Task 3 — both consume only Task 1; independent of each other.
**Phase 3 (sequential):** Task 4 — documents the final shape of Task 2 and Task 3.

## Rough sketch

```sql
CREATE TABLE unsupported_companies (
    id                bigserial PRIMARY KEY,
    name              text NOT NULL,
    url               text,
    detected_platform text,
    reason            text NOT NULL CHECK (reason IN ('unsupported_ats', 'no_careers')),
    first_seen_at     timestamptz NOT NULL DEFAULT now(),
    last_checked_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_unsupported_companies_normalized_name
    ON unsupported_companies (lower(regexp_replace(name, '[^[:alnum:]]', '', 'g')));
```

`mcp.record_unsupported_company(p_name, p_url, p_detected_platform, p_reason)` mirrors `mcp.add_company`'s shape: `INSERT ... ON CONFLICT (<normalized name expression>) DO UPDATE SET last_checked_at = now(), url = COALESCE(EXCLUDED.url, unsupported_companies.url), detected_platform = COALESCE(EXCLUDED.detected_platform, unsupported_companies.detected_platform), reason = EXCLUDED.reason`, returning the row plus a `was_new boolean`.

## Boundary inventory

| Name | Go struct field | JSON key | SQL column |
|---|---|---|---|
| Unsupported record | `dedupUnsupportedRecord` | `"known_unsupported"` (on `dedupCandidateResult`) | — (assembled, not a column) |
| Company/URL name | `Name` | `"name"` | `name` |
| Evidence URL | `URL *string` | `"url"` | `url` |
| Detected platform | `DetectedPlatform *string` | `"detected_platform"` | `detected_platform` |
| Trigger reason | `Reason string` | `"reason"` | `reason` (`CHECK IN ('unsupported_ats', 'no_careers')`) |
| First seen | `FirstSeenAt string` (RFC3339) | `"first_seen_at"` | `first_seen_at` |
| Last checked | `LastCheckedAt string` (RFC3339) | `"last_checked_at"` | `last_checked_at` |
| Revisit flag | `Stale bool` | `"stale"` | — (computed in Go against `dedupUnsupportedRevisitDays`) |

`reason` wire values are exactly `unsupported_ats` and `no_careers` — same casing on the `record_unsupported_ats` request, the SQL `CHECK` constraint, and the `dedup_candidates` response.

## Open questions

None — resolved during drafting:
- Storage shape: a dedicated table, not a column on `companies` (which has no room for an unfetchable row) and not a JSONL sidecar (not queryable by the live MCP path).
- Trigger scope: both `unsupported-ats` (tool-attested, via `detect_ats`'s URL parsing) and `no-careers` (agent-asserted — no tool observes it on the live path today) populate the registry. Per `project.md`'s trust-tier discipline, the two are distinguished by `reason` in the record rather than collapsed into one signal; `watchlist.md` should state which tier each `reason` value carries.
- Revisit threshold: 90 days, independent of the existing 30-day posting-recency window used elsewhere in dedup.
