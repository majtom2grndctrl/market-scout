# ATS Source Timestamps

## Goal

Capture the raw ATS-reported timestamps on every snapshot so future work can detect reposts, distinguish stale postings from fresh ones, and run trend analysis on posting age. Adds two ATS-agnostic, nullable columns to `posting_snapshots` and populates them from Greenhouse today, leaving the existing domain-level `posted_at` semantic untouched.

## Scope

### In scope

- New nullable timestamp columns on `posting_snapshots` capturing the ATS-reported "first published" and "last modified" values verbatim.
- Numbered up/down migration adding the columns.
- Updated `InsertPostingSnapshot` query and regenerated sqlc code so the new columns are written on insert.
- Two new optional fields on `domain.Posting` (one per column).
- Greenhouse adapter populates the new fields from the wire response's `first_published` and `updated_at` strings.
- Adapter parses the wire timestamps as RFC 3339 (using `time.RFC3339Nano` to tolerate fractional seconds); unparseable values fail the fetch (consistent with adapter contract — malformed payloads abort, never silently zero out).
- Fetcher write path forwards the new fields through the existing nullable-time helper.
- Adapter unit test coverage for: both timestamps present, both absent, malformed value rejected.
- Comment in `internal/ats/greenhouse.go` updated to reflect that `first_published` / `updated_at` are now captured into the new columns (still not used to derive `posted_at`).

### Out of scope

- Backfilling existing snapshots from `raw_data`. Older rows simply lack the data; trend queries already handle NULL.
- Re-deriving the existing domain `posted_at` from the new columns. Its semantic stays as-is.
- Lever and Ashby adapters — no new adapter code lands here, even though the column names are chosen to fit them later.
- Location parsing / `locations` column work.
- Indexes on the new columns. Add when a query needs one.
- UI, query, or analytics work that reads the new columns.

## Acceptance criteria

- After migration, `\d posting_snapshots` shows `source_first_published_at` and `source_last_modified_at` as nullable `timestamptz` columns.
- The migration is reversible: down migration drops the new columns and leaves the rest of the table unchanged.
- Given a Greenhouse fixture payload supplying both timestamp fields, the adapter emits a `domain.Posting` with both `*time.Time` fields set, and the fetcher write path forwards them into `InsertPostingSnapshotParams` via `nullTime`.
- A wire payload missing one or both timestamp fields produces a snapshot with NULL in the corresponding column(s) and no error.
- A wire payload with an unparseable timestamp string fails the whole company fetch with a wrapped error naming the offending field; no partial snapshots are written for that company.
- The Greenhouse adapter continues to leave `posted_at` nil. (Lever and Ashby are out of scope; no other adapter files are modified.)
- The `ghJob` block comment names the new fields and explains why neither is promoted to `PostedAt`.
- Pre-existing snapshot rows are left untouched and remain NULL in the new columns.
- `go build ./...`, `go vet ./...`, and `go test ./...` pass.
- After Task 2, re-running `sqlc generate` against committed SQL produces no diff.

## Tasks

### Task 1 — Schema migration

Write `000003_posting_snapshots_source_timestamps` up/down migration. Up migration: `ALTER TABLE posting_snapshots ADD COLUMN source_first_published_at timestamptz, ADD COLUMN source_last_modified_at timestamptz;`. Down migration: matching `DROP COLUMN` pair. Do not modify 000001. Apply locally; confirm `\d posting_snapshots`. Confirm down migration cleanly reverses.

### Task 2 — Query + sqlc regeneration

Update `internal/db/queries/fetcher.sql` so `InsertPostingSnapshot` writes the two new columns. Append both columns at the end so the diff is purely additive and existing `$1..$11` parameter positions are unchanged. Run `sqlc generate`; commit the regenerated files.

### Task 3 — Domain field additions

Add two optional `*time.Time` fields to `domain.Posting`. Update the doc comment to describe their semantic — raw ATS-reported timestamps, distinct from domain `PostedAt`. Insert the new fields between `PostedAt` and `JobURL`. No struct tags (matches surrounding fields). Add per-field line comments (e.g. `// SourceFirstPublishedAt is the ATS-reported first-published timestamp, nil if not supplied.`); the type-level doc already covers nil-as-NULL semantics, do not extend it.

### Task 4 — Greenhouse adapter parsing

Extend `ghJob` with the two wire fields. Go field names and tags: `FirstPublished string \`json:"first_published"\`` and `UpdatedAt string \`json:"updated_at"\``. Parse each as RFC 3339 (`time.RFC3339Nano`); empty/missing → nil; non-empty but unparseable → wrapped error of the form `greenhouse: job at index %d for %s: parse %s %q: %w` where `%d` is the loop index, the first `%s` is the board token (company identity), the second `%s` is the wire field name (`first_published` or `updated_at`), `%q` is the raw unparseable value. Matches the surrounding error style in greenhouse.go. Update the existing block comment that disclaims `first_published` / `updated_at` to reflect the new behavior (captured raw, not used to derive `PostedAt`).

### Task 5 — Fetcher write-site wiring

Map the two new `domain.Posting` fields into `InsertPostingSnapshotParams` via the existing `nullTime` helper.

### Task 6 — Tests

Add Greenhouse adapter cases for: both timestamps present and parsed, both absent producing nil fields, malformed value for `first_published` producing a wrapped error naming that wire field, and separately for `updated_at` — both to verify the `%s` field-name substitution. Add a fetcher-side test in `cmd/fetcher/main_test.go` (no pre-existing snapshot-write coverage) asserting the two new `domain.Posting` `*time.Time` fields flow into `InsertPostingSnapshotParams` via `nullTime`; verify `InsertPostingSnapshotParams.SourceFirstPublishedAt` and `SourceLastModifiedAt` are `sql.NullTime{Valid: true}` with the expected time values.

## Sequencing

Task 1 first. Tasks 2 and 3 may run in parallel after 1. Task 4 depends on 3. Task 5 depends on 2 and 3. Task 6 depends on 4 and 5. Recommended order: 1, 2, 3, 4, 5, 6.

## Design decisions

**Column names.** `source_first_published_at` and `source_last_modified_at`.

Rationale: the `source_*` prefix marks these as raw ATS-reported values (distinct from any domain-derived field), and generalizes cleanly across vendors — Lever exposes `createdAt` / `updatedAt`, Ashby exposes `publishedDate` / `updatedAt`, all of which map to these two semantic slots. `first_published` mirrors Greenhouse's exact wire field name, which is the strongest signal we have for "first version of this posting"; `last_modified` is the generic phrasing for what every ATS calls some variant of `updated_at`. Avoided `source_first_seen_at` because "seen" implies our observation, not the source's claim.

**Types.** Both columns: `timestamptz NULL`. No defaults. Older snapshots and ATSes that don't expose one or both fields persist as NULL; absence is meaningful. Wire fields are typed `string` (not `*string`); JSON null, missing key, and empty string all decode to `""` and are treated uniformly as absent.

**Domain field names.** `SourceFirstPublishedAt` and `SourceLastModifiedAt`, both `*time.Time`. Mirrors the column naming, keeps the contract obvious at the call site.

**Greenhouse parsing.** Greenhouse returns timestamps like `"2024-08-12T17:33:21-04:00"` — RFC 3339 with offset. Use `time.Parse(time.RFC3339Nano, s)`. Empty string → nil. time.RFC3339Nano accepts both numeric offsets and the Z UTC designator, and tolerates fractional seconds; all are valid Greenhouse output forms. Parse failure → wrapped error of the form `greenhouse: job at index %d for %s: parse %s %q: %w`, where `%d` is the loop index, first `%s` is the board token, second `%s` is the wire field name, `%q` is the raw value.

**Existing `posted_at`.** Untouched. Greenhouse adapter continues to leave it nil. The disclaimer comment in `greenhouse.go` is rewritten, not deleted: it now explains *why* we capture the raw timestamps but still don't promote either to `PostedAt`.
