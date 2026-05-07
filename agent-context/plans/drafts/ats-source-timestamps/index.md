# ATS Source Timestamps

## Goal

Capture the ATS-reported timestamps on every snapshot so we can detect reposts, distinguish stale postings from fresh ones, and run trend analysis on posting age. Adds two ATS-agnostic, nullable columns to `posting_snapshots` and populates them from Greenhouse today, leaving the existing domain-level `posted_at` semantic untouched.

## Scope

### In scope

- New nullable timestamp columns on `posting_snapshots` capturing the ATS-reported "first published" and "last modified" values verbatim.
- Numbered up/down migration adding the columns.
- Updated `InsertPostingSnapshot` query and regenerated sqlc code so the new columns are written on insert.
- Two new optional fields on `domain.Posting` (one per column).
- Greenhouse adapter populates the new fields from the wire response's `first_published` and `updated_at` strings.
- Adapter parses the wire timestamps as RFC 3339; unparseable values fail the fetch (consistent with adapter contract — malformed payloads abort, never silently zero out).
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

- After migration, `\d posting_snapshots` shows two new nullable `timestamptz` columns named per Rough sketch.
- The migration is reversible: down migration drops the new columns and leaves the rest of the table unchanged.
- A fresh fetch against a Greenhouse board populates both new columns for every row whose wire payload supplies the corresponding fields.
- A wire payload missing one or both timestamp fields produces a snapshot with NULL in the corresponding column(s) and no error.
- A wire payload with an unparseable timestamp string fails the whole company fetch with a wrapped error naming the offending field; no partial snapshots are written for that company.
- The existing `posted_at` column continues to be written as it is today (NULL from the Greenhouse adapter); behavior unchanged.
- Pre-existing snapshot rows are left untouched and remain NULL in the new columns.
- `go build ./...`, `go vet ./...`, and `go test ./...` pass.
- `sqlc generate` produces no diff after a clean run.

## Tasks

### Task 1 — Schema migration

Write `000003_*` up/down migration adding two nullable `timestamptz` columns to `posting_snapshots`. Apply locally; confirm `\d posting_snapshots`. Confirm down migration cleanly reverses.

### Task 2 — Query + sqlc regeneration

Update `internal/db/queries/fetcher.sql` so `InsertPostingSnapshot` writes the two new columns. Run `sqlc generate`; commit the regenerated files.

### Task 3 — Domain field additions

Add two optional `*time.Time` fields to `domain.Posting`. Update the doc comment to describe their semantic — raw ATS-reported timestamps, distinct from domain `PostedAt`.

### Task 4 — Greenhouse adapter parsing

Extend `ghJob` with the two wire fields. Parse each as RFC 3339 (`time.RFC3339`); empty/missing → nil; non-empty but unparseable → error wrapped with the offending field name and job index. Update the existing block comment that disclaims `first_published` / `updated_at` to reflect the new behavior (captured raw, not used to derive `PostedAt`).

### Task 5 — Fetcher write-site wiring

Map the two new `domain.Posting` fields into `InsertPostingSnapshotParams` via the existing `nullTime` helper.

### Task 6 — Tests

Add Greenhouse adapter cases for: both timestamps present and parsed, both absent producing nil fields, malformed value producing a wrapped error.

## Sequencing

1 → 2 → 3 in any order after 1, but 2 must complete before 5 compiles. 4 depends on 3. 5 depends on 2 and 4. 6 depends on 4. Recommended order: 1, 2, 3, 4, 5, 6.

## Rough sketch

**Column names.** `source_first_published_at` and `source_last_modified_at`.

Rationale: the `source_*` prefix marks these as raw ATS-reported values (distinct from any domain-derived field), and generalizes cleanly across vendors — Lever exposes `createdAt` / `updatedAt`, Ashby exposes `publishedDate` / `updatedAt`, all of which map to these two semantic slots. `first_published` mirrors Greenhouse's exact wire field name, which is the strongest signal we have for "first version of this posting"; `last_modified` is the generic phrasing for what every ATS calls some variant of `updated_at`. Avoided `source_first_seen_at` because "seen" implies our observation, not the source's claim.

**Types.** Both `timestamptz NOT NULL`-able — i.e. nullable. Older snapshots and ATSes that don't expose one or both fields persist as NULL. No defaults; absence is meaningful.

**Domain field names.** `SourceFirstPublishedAt` and `SourceLastModifiedAt`, both `*time.Time`. Mirrors the column naming, keeps the contract obvious at the call site.

**Greenhouse parsing.** Greenhouse returns timestamps like `"2024-08-12T17:33:21-04:00"` — RFC 3339 with offset. Use `time.Parse(time.RFC3339, s)`. Empty string → nil. Parse failure → wrapped error including job index, field name, and the bad value (truncated if long).

**Existing `posted_at`.** Untouched. Greenhouse adapter continues to leave it nil. The disclaimer comment in `greenhouse.go` is rewritten, not deleted: it now explains *why* we capture the raw timestamps but still don't promote either to `PostedAt`.

## Open questions

- Are `first_published` and `updated_at` always present on Greenhouse responses in practice, or do some boards omit them? The spec already covers absence (NULL), so the answer only affects how loud the test fixtures need to be — not behavior.
