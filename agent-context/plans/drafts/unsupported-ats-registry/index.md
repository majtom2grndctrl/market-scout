# Unsupported ATS Registry

## Goal

Persist companies with a known-unsupported ATS, or no discoverable careers page, so `dedup_candidates` can warn agents off re-investigating them in the browser. Surfaced as informational metadata only — never a hard skip — consistent with the trust-tier discipline the rest of `dedup_candidates` already holds itself to. Each `reason` carries its own tier (see Task 4): `unsupported_ats` is Deterministic, `no_careers` is Agent-asserted.

## Scope

### In scope
- New table `unsupported_companies` and its write path: `mcp.record_unsupported_company` (SECURITY DEFINER function) plus a `record_unsupported_company` MCP action tool, mirroring `add_company`'s write-gate pattern.
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
- [ ] `mcp.record_unsupported_company` inserts a new row on first sight of a normalized name, and on a repeat sighting overwrites `url`, `detected_platform`, and `reason` with the incoming call's values and bumps `last_checked_at` to now. `name` and `first_seen_at` are preserved from the first sighting and never overwritten.
- [ ] `record_unsupported_company` validates: `name` required; `reason` is exactly `unsupported_ats` or `no_careers`; `url` required when `reason` is `unsupported_ats`, optional when `no_careers`; a syntactically invalid `url` is a validation error, not a silently dropped field.
- [ ] `dedup_candidates` returns `known_unsupported: null` by default, populated for any candidate that did not resolve via token match and whose normalized name or URL host matches a row in `unsupported_companies`.
- [ ] A populated `known_unsupported` includes `name`, `url` (nullable), `reason`, `detected_platform` (nullable), `first_seen_at`, `last_checked_at`, and `stale` (`true` when `last_checked_at` is more than 90 days old).
- [ ] `known_unsupported` never changes `verdict`, `match_kind`, `matches`, or `match_count` for the same result — a candidate can be `verdict: "new"` and still carry a populated `known_unsupported`.
- [ ] When a candidate's lookup matches `unsupported_companies` by both URL host and name, the host match wins — mirrors the existing `domain > name_only` priority for company matches. When more than one stored row shares a host, the most recently checked row wins.
- [ ] `record_unsupported_company` and the two new dedup lookup queries each have an `integration`-tagged test against a live Postgres instance.
- [ ] `discovery-run` calls `record_unsupported_company` when `detect_ats` returns `unsupported-ats`, and when browser investigation concludes `no-careers`, instead of only reporting the status and dropping it.
- [ ] `watchlist.md` documents what triggers a registry write, the 90-day revisit threshold, the trust tier each `reason` carries, and that `known_unsupported` informs but never gates an agent's decision to re-investigate.

## Tasks

### Task 1: Unsupported companies table and write path

- Add a migration creating `unsupported_companies` and a functional unique index on the same normalization `FindCompaniesByNormalizedNames` uses. Reusing it keeps write-time and read-time identity checks consistent.

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

  Pair it with a `.down.sql` dropping the index and table, matching every other migration in `internal/db/migrations/`.
- Add `mcp.record_unsupported_company` as a SECURITY DEFINER function in a companion migration. Harden it exactly like `mcp.add_company` (see Mirror table) — the same threat model applies, since both are write functions the action role invokes.

  ```sql
  INSERT INTO public.unsupported_companies (name, url, detected_platform, reason)
  VALUES (p_name, p_url, p_detected_platform, p_reason)
  ON CONFLICT (lower(regexp_replace(name, '[^[:alnum:]]', '', 'g')))
  DO UPDATE SET
      url               = EXCLUDED.url,
      detected_platform = EXCLUDED.detected_platform,
      reason            = EXCLUDED.reason,
      last_checked_at   = now()
  RETURNING id, name, url, detected_platform, reason, first_seen_at, last_checked_at
  ```

  No `was_new` flag in the response: `add_company`'s insert/conflict distinction relies on `ON CONFLICT DO NOTHING`'s all-or-nothing `RETURNING`, which doesn't carry over to `DO UPDATE` (every call returns a row), and nothing in this spec's AC needs to distinguish first-sight from repeat-sight at the response layer. The success response returns exactly the seven columns the `RETURNING` clause above produces.
- On conflict against the unique index, overwrite `url`, `detected_platform`, and `reason` with the incoming call's values and bump `last_checked_at` to `now()` — do not merge with the stored row. Merging would let a company re-sighted as `no_careers` keep a stale `url` / `detected_platform` from an earlier `unsupported_ats` sighting. Do not overwrite `name` or `first_seen_at` on conflict — they identify and date the first sighting.
- This function has a multi-column `RETURNS TABLE`, which sqlc's offline parser cannot expand without a live database (`developer-guide.md` §5.7) — the same limitation `add_company` already works around. Call it with a fixed, fully-parameterized `QueryRowContext`, not a sqlc query file.
- Add the MCP action tool `record_unsupported_company` in a new `apps/tools/cmd/mcp/record_unsupported_company.go`, registered in `main.go` bound to `pools.action` — it writes through an approved function, same reasoning as `add_company`'s pool binding.
- Request fields (see table below). Validate `url` with `atsdetect.ValidateURL` when present.

| Field | Required | Allowed values |
|---|---|---|
| `name` | Always | Free text |
| `reason` | Always | `unsupported_ats` or `no_careers` |
| `url` | When `reason` is `unsupported_ats` | Valid URL |
| `detected_platform` | Never | Free text |

- Name the Go constant backing the `"unsupported_ats"` reason value distinctly from `add_company.go`'s existing `codeUnsupportedATS` — the two share a string but different meanings (an action error code vs. a stored reason), and reusing the const would conflate them.
- Add an `integration`-tagged test against a live Postgres instance covering `record_unsupported_company`'s insert and conflict-update paths. This calls the function through the action pool, so `internal/db/setup/action_role.sql` must be rerun against the test database after the new migration applies — same operational step `add_company`'s grant already requires (`developer-guide.md` §2).

Mirror table:

| Mirror | Don't mirror |
|---|---|
| `mcp.add_company` — SECURITY DEFINER hardening (owner-run, `SET search_path = pg_catalog`, `REVOKE ALL ... FROM PUBLIC`, explicit grant in `apps/tools/internal/db/setup/action_role.sql`) | `mcp.add_company`'s `ON CONFLICT ... DO NOTHING` — this function needs `DO UPDATE`, since a repeat sighting must refresh the row |
| `poolExecutor` / `addCompanyExecutor` in `add_company.go` — injectable-executor test seam | — |
| `add_company.go`'s envelope shape (`Ok`, `Errors`) | — |

Do not:
- Route this write through the read-only query gateway, or generate it as a sqlc function.
- Store this data in `companies` or reuse `mcp.add_company` — it is a separate registry with a separate lifecycle (see Scope).

### Task 2: Surface known_unsupported in dedup_candidates

- Add `FindUnsupportedByNames(ctx, names []string) (map[int]dedupUnsupportedRecord, error)` to `dedupSource`, mirroring `FindByNames`'s shape. Back it with a new sqlc `:many` query in `onboard.sql` reusing `FindCompaniesByNormalizedNames`'s normalization expression. The unique index from Task 1 guarantees at most one row per candidate.
- Add `FindUnsupportedByURLHost(ctx, urls []string) (map[int]dedupUnsupportedRecord, error)` to `dedupSource`, backed by a new sqlc `:many` query reusing `FindCompaniesByCareersURLHost`'s host-extraction expression. `unsupported_companies` has no unique constraint on host, so more than one row can share one — use `DISTINCT ON (input_index) ... ORDER BY input_index, last_checked_at DESC`, the same "latest row wins" pattern `developer-guide.md` §6.2 documents for `classifications`, so the most recently checked row wins per candidate. Both new queries are plain `:many` SELECTs, not `RETURNS TABLE` functions, so sqlc's offline parser generates them the same way it already generates `FindCompaniesByNormalizedNames` and `FindCompaniesByCareersURLHost` — no `add_company`-style hand-written `QueryRowContext` needed here.
- `dedup_candidates` reads through `pools.readOnly`, but the new table needs no extra grant: `internal/db/setup/readonly_role.sql`'s `ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES` (`readonly_role.sql` §Read-only MCP role) covers any table created after the script last ran, as long as the migration runs under the same owner role used for setup — no rerun required, unlike the action role's per-function grant in Task 1.
- In `runDedupCandidates`, call both against the `nameLookupNames` / `domainLookupURLs` batches already gathered for company matching — no new input-gathering logic needed. Run this only for candidates that did not resolve via token match; a token match means the company is already tracked, so it cannot also be in `unsupported_companies`.
- When both lookups hit for the same candidate, the URL-host result wins — mirrors the existing `domain > name_only` priority for company matches.
- Add `KnownUnsupported *dedupUnsupportedRecord` (`json:"known_unsupported"`) to `dedupCandidateResult`, and a new `dedupUnsupportedRecord` struct:

  | Go field | Type | JSON key | Nullable |
  |---|---|---|---|
  | `Name` | `string` | `"name"` | no |
  | `URL` | `*string` | `"url"` | yes |
  | `DetectedPlatform` | `*string` | `"detected_platform"` | yes |
  | `Reason` | `string` | `"reason"` | no |
  | `FirstSeenAt` | `string` (RFC3339) | `"first_seen_at"` | no |
  | `LastCheckedAt` | `string` (RFC3339) | `"last_checked_at"` | no |
  | `Stale` | `bool` | `"stale"` | no (computed, not a column) |

  Compute `Stale` in `runDedupCandidates` by parsing `LastCheckedAt` and comparing its age against a new constant `dedupUnsupportedRevisitDays = 90`, placed near `dedupFuzzyNameSimilarityThreshold`. `LastCheckedAt` stays an RFC3339 string — parse it at the point `Stale` is computed, don't add a second timestamp field.
- Update `fakeDedupSource` in `dedup_candidates_test.go` to implement `FindUnsupportedByNames` and `FindUnsupportedByURLHost`, returning empty maps by default so existing unit tests keep passing unchanged.
- Add an `integration`-tagged test against a live Postgres instance covering both new queries, including the host tie-break when two rows share a host.

Do not:
- Let a populated `known_unsupported` change `Verdict`, `MatchKind`, `Matches`, or `MatchCount` — set it independently of the existing match-merging logic in `mergeDedupMatches`.
- Compute the 90-day staleness window in SQL. It is a fixed constant, unlike the caller-configurable `recency_days` window the rest of the tool uses.

### Task 3: Wire discovery-run to record unsupported outcomes

- In `.claude/skills/discovery-run/SKILL.md`, add `mcp__market-scout-postgres__record_unsupported_company` to the Required-tools list at the top of the skill, alongside `dedup_candidates`, `detect_ats`, and `add_company` — the skill stops if a required tool isn't connected, and this tool is now load-bearing for two outcomes.
- After `detect_ats` returns `unsupported-ats`, call `record_unsupported_company` with the candidate's `name`, `reason: "unsupported_ats"`, and the URL evidence that produced the result, before recording the candidate's final status.
- When browser investigation finds no usable careers page or board at all (today's `no-careers` status), call `record_unsupported_company` with the candidate's `name`, `reason: "no_careers"`, the observed homepage URL if one was found (otherwise omit `url`), and no `detected_platform`.
- Add a line to the Safety rules section: call `record_unsupported_company` for both outcomes so later runs don't repeat the same browser investigation.

Do not:
- Call `record_unsupported_company` for `invalid-token` or `ambiguous` outcomes — those mean a supported ATS was found but needs a different token or disambiguation, not that the ATS itself is unsupported.

### Task 4: Document the registry

- In `watchlist.md` §Dedup, document `known_unsupported`: what it is, that it is informational only, and that it never changes verdict or match_kind.
- Add a new subsection documenting the registry itself: what triggers a write (`unsupported-ats` from `detect_ats`, `no-careers` from browser investigation), the fields captured, and the 90-day revisit threshold — after which `stale: true` signals the agent that a re-check may be worthwhile, not that one is required.
- State the trust tier each `reason` carries: `unsupported_ats` is Deterministic (from `detect_ats`'s URL parsing); `no_careers` is Agent-asserted (no tool observes it on the live path). A future agent reading `known_unsupported` should weigh a `no_careers` record as one prior agent's browser judgment, not a verified fact.

## Sequencing

**Phase 1 (sequential):** Task 1 — defines the table, the write function, and the tool the other tasks assume exist.
**Phase 2 (concurrent):** Task 2, Task 3 — both consume only Task 1; independent of each other.
**Phase 3 (sequential):** Task 4 — documents the final shape of Task 2 and Task 3.

## Rough sketch

Inlined into Task 1: migration DDL and the `mcp.record_unsupported_company` `ON CONFLICT DO UPDATE` statement.

## Boundary inventory

Inlined into Task 2: the `dedupUnsupportedRecord` field / JSON-key / type table.

`reason` wire values are exactly `unsupported_ats` and `no_careers` — same casing on the `record_unsupported_company` request, the SQL `CHECK` constraint, and the `dedup_candidates` response.

## Open questions

None — resolved during drafting:
- Storage shape: a dedicated table, not a column on `companies` (which has no room for an unfetchable row) and not a JSONL sidecar (not queryable by the live MCP path).
- Trigger scope: both `unsupported-ats` (tool-attested, via `detect_ats`'s URL parsing) and `no-careers` (agent-asserted — no tool observes it on the live path today) populate the registry, distinguished by `reason` rather than collapsed into one signal. Task 4 / AC require `watchlist.md` to state each `reason`'s trust tier so a future agent doesn't read a `no_careers` record as verified fact.
- Revisit threshold: 90 days, independent of the existing 30-day posting-recency window used elsewhere in dedup.
- Host-match tie-break: `unsupported_companies` has no unique constraint on URL host (only on normalized name), so a host match uses "most recently checked row wins" rather than surfacing every match — unlike company matching, this registry has no human-disambiguation use case that needs to see collisions.
