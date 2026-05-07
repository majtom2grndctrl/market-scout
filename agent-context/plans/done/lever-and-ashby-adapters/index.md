# Lever and Ashby Adapters

## Goal

Land Lever and Ashby ATS adapters together so the snapshot schema evolves once. Bring trend-relevant fields the existing schema doesn't yet capture (true posted-at, multi-market locations, employment type, workplace type) into normalized columns chosen from the query side — not from any one source's wire shape. Defend our own first-seen signal as the load-bearing repost detector. Keep the door open for non-ATS sources next.

## Context

Three pressures triangulate this spec:

- **Existing schema.** `job_postings` (identity), `posting_snapshots` (per-fetch row, append-only), with `source_first_published_at` / `source_last_modified_at` already added per the in-progress source-timestamps spec. `source_type` already accepts `'ats' | 'web' | 'agent'`.
- **Source shapes.** Lever (createdAt ms epoch, lowercase workplaceType, free-text `commitment`, `categories.allLocations`), Ashby (publishedAt ISO, PascalCase WorkplaceType, enum `employmentType`, structured `address` + `secondaryLocations`), Greenhouse (no posted_at, no employment type, no workplace type, single `location.name`). See `research/ats-apis-research.md`.
- **Use case.** Personal market-intel tool tracking hiring trends across companies of interest. The query side asks: "Which companies are hiring remote senior designers in Q1?" "Are postings getting reposted more often?" "Which companies opened markets they weren't hiring in last quarter?" The use case supersedes exact matches with sources.

## Design decisions

### Repost detection rests on `job_postings.first_seen_at`, not source timestamps

ATS-reported "first published" and "created at" fields are suspected to refresh when a role is reposted on the same board. They cannot be trusted as repost signals on their own.

The load-bearing repost signal is **our own** first-seen timestamp (`job_postings.first_seen_at`) paired with **gap detection** across `posting_snapshots.fetched_at` for the same `job_posting_id`. A posting that disappears for one or more fetches and reappears with the same `(company_id, source_url)` is a continuation; a new `source_url` for a previously-seen role is a new posting. Source-reported timestamps still go on every snapshot per the in-progress source-timestamps work — they're useful as a secondary signal (did the source's claimed posted_at change between snapshots?) but never as the primary.

Concretely:

- `job_postings.first_seen_at` is **promoted in documentation** to "the load-bearing first-seen signal." The column already exists; this spec elevates its meaning, doesn't change its shape.
- Repost detection is not implemented in this spec. The schema must support it: snapshots must remain append-only, gaps must be inferable from `(job_posting_id, fetched_at)`, and `first_seen_at` must remain immutable across snapshot writes (the existing `UpsertJobPosting` query already preserves it — `ON CONFLICT DO UPDATE SET source_id = EXCLUDED.source_id` does not touch `first_seen_at`).
- Reposts under a *new* `source_url` (the ATS minted a fresh ID) are correctly modeled as a new `job_postings` row with a new `first_seen_at`. Joining reposts back to a logical role is a downstream concern (NLP / fuzzy match on title + company + location), not a schema concern.

### `posted_at` semantics: keep one normalized column, derive best-effort

Three sources, three answers: Greenhouse has no reliable posted-at, Lever has true `createdAt`, Ashby has `publishedAt`. Options weighed:

1. Per-source columns (`lever_created_at`, `ashby_published_at`). Rejected — bakes ATS names into the schema; query side has to know which column to read per company.
2. Drop `posted_at` entirely; rely only on `first_seen_at`. Rejected — wastes the genuine signal Lever and Ashby provide for postings older than our first observation.
3. Keep one normalized `posted_at` column, populated best-effort, NULL when the source doesn't expose one. **Chosen.**

`posted_at` carries source-claimed posting age, NULL when unknown. Trend queries treat NULL as "use `first_seen_at` instead." `source_first_published_at` and `source_last_modified_at` continue to capture per-snapshot raw values for change detection. The Greenhouse adapter continues to leave `posted_at` NULL (already its behavior).

### Workplace type: enum only, normalize at adapter boundary

`workplace_type` enum (`onsite | hybrid | remote`) is already in the schema. Normalization rule: lowercase the wire string, match against `{onsite, hybrid, remote}`; Lever's `unspecified` and any unrecognized value → nil. This handles Lever's already-lowercase strings and Ashby's PascalCase (`"OnSite"`, `"Remote"`, `"Hybrid"`) uniformly. The column stores only the enum values.

**`is_remote` is dropped from consideration.** The research doc proposed it; user is not interested. `workplace_type = 'remote'` answers the same question without redundancy.

### Employment type: normalize at adapter boundary, log unknowns

Lever returns free text (`"Full-time"`, `"Contract"`, `"Part time"`, capitalization varies); Ashby returns an enum (`"FullTime"`, `"PartTime"`, `"Intern"`, `"Contract"`, `"Temporary"`); Greenhouse omits the field. The schema's enum (`full_time | part_time | contract | intern | temporary`) is fixed.

Normalization lives **inside each adapter**. The rule: lowercase the wire string, strip non-alphanumeric characters, match against a per-adapter alias map. Unknown values produce NULL plus a one-line `slog.Warn` tagged with the wire value, so genuinely-new values surface without aborting the fetch. A separate "mapping layer" package is rejected as premature — three adapters and a fixed schema enum don't earn a package boundary. Promote to a shared helper if a fourth source repeats the pattern.

Lever starter alias map (extend via warns as real-world strings surface):

| Schema value | Lever aliases (post-normalize) |
|---|---|
| `full_time` | `fulltime`, `ft` |
| `part_time` | `parttime`, `pt` |
| `contract` | `contract`, `contractor` |
| `intern` | `intern`, `internship` |
| `temporary` | `temporary`, `temp` |

If the warn channel turns up many unmapped values in practice, the response is to extend the schema enum (a migration) or extend the per-adapter mapping (a code change) — not to invent a new abstraction.

### Locations: compound field today, room for NLP later

The query side wants to ask "which companies are hiring in NYC?" and have a posting open to "SF Bay Area / NYC" match. Lever's `categories.allLocations` and Ashby's `secondaryLocations` both expose multi-market roles; Greenhouse does not.

**Schema choice: a `text[]` column on `posting_snapshots` capturing all source-claimed location strings, plus the existing scalar `location_text`.** Naming: `location_texts text[]` (plural). Greenhouse populates it as a single-element array from `location.name`; Lever from `categories.allLocations` (falling back to `[categories.location]` when allLocations is absent); Ashby from `[location] ++ secondaryLocations[].postalAddress` rendered to display strings.

`location_text` (scalar) stays as the primary display string; `location_texts` (array) is the compound field. Both are populated: `location_text` = first element of `location_texts` where applicable. Query side joins `unnest(location_texts)` to filter by market.

**NLP layer is deferred.** When it lands, it produces normalized output (e.g. canonical city/region tokens). The normalized output goes in a *new* column or table at that point — not in this spec. Two paths are kept open:

- New columns on `posting_snapshots` (e.g. `cities_normalized text[]`, `regions_normalized text[]`) if normalization is cheap and per-snapshot.
- A separate `posting_snapshot_locations` table joined to snapshots, if normalization carries metadata (confidence, source, parser version).

Either works without rewriting `location_texts`. The constraint this spec defends: never overwrite or replace the raw source strings — store them verbatim. Normalization is additive, downstream, and explicitly out of scope here.

The existing `job_postings.cities text[]` column predates this work. It's left alone in this spec — its semantics are unclear and it's currently unused by the fetcher. A follow-up should decide whether to deprecate it once `location_texts` proves out.

### SourceURL vs JobURL: identity vs display

`SourceURL` is the canonical identity URL — used as the `(company_id, source_url)` uniqueness key for `job_postings`. `JobURL` is the human-facing posting page when a distinct one exists. For Lever and Ashby, one URL serves both purposes: set them equal, using the source's canonical posting URL. Do not synthesize an API URL for `SourceURL` on these adapters. Greenhouse keeps its synthesized API URL as `SourceURL` (existing behavior, unchanged).

- Lever: `SourceURL` and `JobURL` both = `hostedUrl`.
- Ashby: `SourceURL` and `JobURL` both = `jobUrl`.

### Adapter abstraction: stay structural for now, plan the rename

Today there's no shared interface in `internal/ats`; the Greenhouse adapter satisfies an unexported `atsAdapter` interface in `cmd/fetcher` via structural typing. With three concrete adapters plus future web sources, two questions:

1. **Promote the interface to a shared package?** Not yet. The interface has one method (`FetchPostings(ctx, boardToken) ([]Posting, error)`) and three implementors of the same shape. Defining it in `cmd/fetcher` is idiomatic Go (interfaces in the consumer) and the cost of adding a Lever or Ashby file is unchanged either way. Revisit when a non-ATS source lands and the consumer-side method set actually diverges.

2. **Lift HTTP boilerplate into a shared helper.** Yes — three adapters will repeat the same body-cap + error-snippet + status-check pattern. Extract `internal/ats/httpfetch.go` (or similar; package-private name) exposing one helper that performs `GET + status check + bounded body read` and returns the raw bytes plus a wrapped error. Greenhouse migrates onto it as part of this spec; Lever and Ashby use it from day one. The 32 MiB body cap and 4 KiB error snippet cap stay constants in the helper.

3. **Lever pagination.** Per the research doc, Lever exposes `limit` (1–100) plus an opaque `offset` token with `next` / `hasNext` in the response. The research is internally inconsistent on this (it also describes the wrapper as a bare array), so Task 5 begins with a live-API spike to pin the actual shape. Whatever the spike finds, the principle holds: if pagination is needed, the loop lives inside the Lever adapter — not a shared helper. With at most one paginated source, abstraction is premature. The shared HTTP helper handles a single request; the Lever adapter calls it once or in a loop as appropriate.

### Generalizing beyond ATS: rename plan, not a rename now

`job_postings.source_type` already accepts `'ats' | 'web' | 'agent'`. The adapter package name (`internal/ats`) bakes in ATS-ness and *will* feel wrong when the first web scraper lands. Options:

1. Rename `internal/ats` → `internal/sources` now. Rejected — premature; no concrete web source yet, and the rename touches every adapter file and import.
2. Defer the rename until the first non-ATS source lands. **Chosen.** When that source arrives, a focused refactor renames the package and moves files. The shared `httpfetch.go` helper is named generically (no `ats` in the name) so it carries forward unchanged.

Schema-side: nothing here assumes ATS. `posting_snapshots` columns are source-shape-agnostic; `source_type` discriminates at query time.

### Loose coupling: defend each column from the query side

Every normalized column added by this spec answers a concrete trend query:

| Column | Trend query it serves |
|---|---|
| `posted_at` (existing, now populated) | "How long has this role been open before we started watching?" |
| `workplace_type` (existing, now populated) | "Of the postings opened this month, what share are remote-only?" |
| `employment_type` (existing, now populated) | "Are companies shifting from FTE to contract hiring?" |
| `location_texts` (new) | "Which companies opened the NYC market this quarter?" — multi-market roles must match. |
| `team` (existing, now populated by both new adapters) | "Which teams are hiring most aggressively at company X?" |

`source_id`, `source_url`, `source_first_published_at`, `source_last_modified_at` serve identity, change detection, and source-roundtrip needs. No column lands without a query that reads it.

Fields kept only in `raw_data`: HTML descriptions, salary ranges, Ashby `address` structured fields, Lever `lists` and `additional`, requisition IDs, language codes, Ashby compensation tiers. All retrievable via JSONB if a query later needs them.

## Scope

### In scope

- New Lever adapter at `internal/ats/lever.go`. Public board base URL, no auth, full normalization into `domain.Posting`. Pagination behavior pinned by the Task 5 live-API spike.
- New Ashby adapter at `internal/ats/ashby.go`. Public posting-api base URL, no auth, no pagination, full normalization.
- Shared HTTP helper in `internal/ats` (file-level, package-private) lifting the GET + status check + bounded body read pattern. Greenhouse migrates onto it in the same spec.
- Schema migration adding `location_texts text[]` to `posting_snapshots`. Nullable. No default. NULL means the adapter did not supply locations; an empty array means the adapter ran but the source returned none. The two are distinct.
- Updated `InsertPostingSnapshot` SQL and regenerated sqlc to write `location_texts`.
- New optional `LocationTexts []string` field on `domain.Posting`. nil → DB NULL; empty slice → DB empty array; populated → DB array.
- Greenhouse adapter populates `LocationTexts` as a single-element array from `location.name` (or nil if `location.name` is empty). Maintains parity with the new column from day one — no Greenhouse rows with NULL `location_texts` while Lever and Ashby populate it.
- Rename of existing `internal/ats.New` → `NewGreenhouse`, with the corresponding call-site update in `cmd/fetcher/main.go` (one line).
- Fetcher dispatch map registers `"lever"` and `"ashby"` adapters alongside `"greenhouse"`.
- Documentation update to `agent-context/lib/project.md` (or a new section there) elevating `job_postings.first_seen_at` as the load-bearing first-seen / repost-detection signal, paired with snapshot gap detection. This is durable architecture, not implementation detail.
- Comment in `internal/ats/greenhouse.go` updated to note `location_texts` population.
- Per-adapter unit tests (httptest.Server + recorded fixtures) covering: full parse, missing optional fields → NULL, multi-page response (Lever only, conditional on spike), non-2xx → wrapped error, malformed JSON → wrapped error, body-cap overflow → wrapped error at the shared helper level (per-adapter tests assert the error surfaces with the adapter's subsystem prefix), unknown employment-type wire value → NULL plus warn, unknown workplace-type wire value → NULL plus warn, multi-market locations populating `LocationTexts`.
- Fetcher write-site test covering `LocationTexts` round-trip via the existing `buildSnapshotParams` test pattern.
- Subsystem log tags `[lever]` and `[ashby]` matching the existing `[greenhouse]` convention.

### Out of scope

- Repost detection logic, queries, or UI. Schema supports it; logic ships separately.
- NLP / location normalization. New columns or tables for normalized output land when the NLP layer lands.
- Renaming `internal/ats` to `internal/sources`. Deferred until the first non-ATS source.
- Promoting the adapter interface to a shared package. Stays consumer-defined in `cmd/fetcher`.
- Backfilling `location_texts` on existing snapshots from `raw_data`. Old rows stay NULL; trend queries handle NULL.
- Deprecating or repurposing `job_postings.cities`. Follow-up.
- Lever `salaryRange` / Ashby compensation as normalized columns. Stays in `raw_data`.
- A shared employment-type / workplace-type mapping package. Lives per-adapter until a fourth source repeats the pattern.
- Any change to `job_postings.first_seen_at`'s shape, default, or write path. The column stays as-is; only documentation changes.
- HTML / plain-text description columns. Stays in `raw_data`.
- Web-source adapters (`source_type = 'web'`). Schema accommodates; no implementation here.
- Index additions on the new `location_texts` column. Add when a query needs one.

## Acceptance criteria

- `\d posting_snapshots` after migration shows `location_texts` as a nullable `text[]` column.
- Down migration runs cleanly without error. Data in `location_texts` is lost on down (acceptable; this is a personal pre-1.0 tool).
- The Lever adapter paginates via `?skip=N&limit=100` (per the Task 5 spike — bare-array responses, no `hasNext` envelope). A multi-page fixture produces one combined `[]domain.Posting` slice and pagination terminates when a page returns fewer than 100 rows.
- A Lever fixture with `categories.allLocations` populated produces `LocationTexts` matching the array.
- A Lever fixture without `categories.allLocations` but with `categories.location` produces `LocationTexts` as a single-element array with that string.
- A Lever fixture with `workplaceType: "remote"` produces `WorkplaceType = "remote"`. `"unspecified"` produces nil. An unrecognized value produces nil plus a `[lever]` warn line naming the wire value.
- A Lever fixture with `categories.commitment: "Full-time"` (and `"full time"`, `"FT"`) produces `EmploymentType = "full_time"` — these illustrate the normalization rule (lowercase + strip non-alphanum + alias match), not an exhaustive alias list. An unrecognized commitment string produces nil plus a `[lever]` warn line.
- A Lever fixture with `createdAt: 1714521600000` produces `PostedAt` set to the corresponding UTC time. Missing `createdAt` produces nil.
- An Ashby fixture with `secondaryLocations` populated produces `LocationTexts` as `[location] ++ rendered(secondaryLocations)`.
- An Ashby fixture with `workplaceType: "Remote"` produces `WorkplaceType = "remote"` (case-folded). Same for `"OnSite"` → `"onsite"`, `"Hybrid"` → `"hybrid"`.
- An Ashby fixture with `employmentType: "FullTime"` produces `EmploymentType = "full_time"`. All five Ashby enum values map cleanly; an unrecognized value produces nil plus a `[ashby]` warn line.
- An Ashby fixture with `publishedAt: "2026-04-15T10:00:00Z"` produces `PostedAt` set to that UTC time. Missing produces nil.
- For both adapters: non-2xx responses, malformed JSON, body-cap overflow, missing `id` all produce wrapped errors that abort the company's fetch (no partial snapshots).
- Greenhouse adapter populates `LocationTexts` as a single-element array from `location.name`; absent/empty `location.name` produces nil.
- No `client.Do` or `http.Get` call exists in `internal/ats/lever.go`, `internal/ats/ashby.go`, or `internal/ats/greenhouse.go` outside the shared helper file.
- Fetcher run completes successfully against a watchlist mixing Greenhouse, Lever, and Ashby companies. Per-company error isolation still holds (a Lever 5xx does not abort an Ashby fetch).
- `agent-context/lib/project.md` (or its `Settled architecture` section) names `job_postings.first_seen_at` as the load-bearing first-seen signal, with one sentence explaining why source timestamps are not trusted for repost detection.
- `go build ./...`, `go vet ./...`, `staticcheck ./...`, and `go test ./...` pass.
- Re-running `sqlc generate` against committed SQL produces no diff.

## Tasks

### Task 1 — Schema migration

Add migration files `000004_posting_snapshots_location_texts.up.sql` and `000004_posting_snapshots_location_texts.down.sql` adding `location_texts text[]` (nullable) to `posting_snapshots`. Down migration drops the column. Apply locally; verify `\d posting_snapshots`.

### Task 2 — Query update + sqlc regeneration

Extend `InsertPostingSnapshot` to write `location_texts`. Append the column at the end of the column list and the parameter list so existing positional parameters are unchanged. Regenerate sqlc, commit the diff. The sqlc array mapping for `text[]` should yield a Go `[]string` parameter; if it instead yields a driver-specific array type, document the wrapper at the call site and proceed — per developer-guide §1.2, this is implementation-time-pinned.

### Task 3 — Domain field addition

Add `LocationTexts []string` to `domain.Posting`. Place it next to `LocationText` in the struct. Document the nil-vs-empty distinction on the field's line comment: nil = source did not supply locations, empty slice = source returned an explicit empty array (rare but distinct), populated = one or more strings. Update the `Posting` doc comment if the existing nil-as-NULL paragraph needs to extend to slices; otherwise leave it.

### Task 4 — Shared HTTP helper

Add a package-private file in `internal/ats` exposing one helper: takes `(ctx, *http.Client, url string)`, returns `([]byte, error)`. The helper performs the GET, checks status, applies the body cap (32 MiB, with the existing `+1` overflow detection), and wraps errors prefixed `httpfetch: ...` with the URL but no caller-domain context. Constants `maxResponseBytes` and `maxErrBodyBytes` move into this file. Migrate Greenhouse onto it; the Greenhouse-specific error prefix (`greenhouse: ...`) is added by the adapter via `fmt.Errorf` wrapping the helper's error. Adapters re-wrap with their subsystem prefix and `boardToken`, e.g. `fmt.Errorf("greenhouse: fetching %s: %w", boardToken, err)`. The board-token context lives in the adapter wrap, not the helper.

### Task 5 — Lever adapter

Write `internal/ats/lever.go`. Constructor `NewLever(client *http.Client) *Lever` matching the Greenhouse signature.

**Spike findings (2026-05-06, against `leverdemo`):** the v0 public postings API returns a **bare JSON array** of jobs — no `{data, next, hasNext}` envelope (research doc was wrong on this). Pagination is by **`?skip=N&limit=100`** (numeric, not opaque tokens; `offset=N` is silently ignored on this API). There is no `hasNext` field; the termination signal is "fewer than `limit` rows returned." The 390-job demo board paginates correctly under this scheme.

`FetchPostings(ctx, boardToken)` paginates with `?limit=100&skip=N&mode=json`, advancing `skip` by 100 each loop, terminating when the API returns fewer than 100 rows. Wire structs are private; one `leverJob` struct mirrors the subset of fields normalized into `domain.Posting`, decoded from per-job `json.RawMessage` so `RawData` preserves the original bytes. Unknown employment-type and workplace-type values warn and produce nil. `createdAt` (ms epoch) parses via `time.UnixMilli`; the same value populates `PostedAt` and `SourceFirstPublishedAt`. `SourceLastModifiedAt` stays nil — Lever exposes no last-modified field. `LocationTexts` falls back from `categories.allLocations` to `[categories.location]` to nil. `LocationText` is set to the first element of `LocationTexts` when non-empty. `SourceURL` and `JobURL` both = `hostedUrl`; reject empty `id` and empty `hostedUrl` per the existing `domain.Posting` contract. Employment-type normalization: lowercase + strip non-alphanum, match starter alias map (extend via warns).

### Task 6 — Ashby adapter

Write `internal/ats/ashby.go`. Constructor `NewAshby(client *http.Client) *Ashby`. Single-request fetch (no pagination) against `https://api.ashbyhq.com/posting-api/job-board/{clientname}`. Response is `{jobs: [...]}` (parallels Greenhouse, not Lever). Wire structs private. Workplace-type normalization: lowercase the wire string, match against `{onsite, hybrid, remote}` (handles PascalCase `"OnSite"`, `"Remote"`, `"Hybrid"` uniformly per the rule in Design decisions). Employment-type mapping:

| Wire value | Schema value |
|---|---|
| `FullTime` | `full_time` |
| `PartTime` | `part_time` |
| `Contract` | `contract` |
| `Intern` | `intern` |
| `Temporary` | `temporary` |

Unknown values warn and produce nil. `publishedAt` parses as RFC 3339. `LocationTexts` is `[location] ++ rendered(secondaryLocations)`: `location` is a string field appended as-is; each `secondaryLocations[i].postalAddress` renders to a display string by joining its `addressLocality`, `addressRegion`, `addressCountry` (skipping empty parts, comma-separated). Skip empty results — the array contains only non-empty strings. `LocationText` is set to the first element of `LocationTexts` when non-empty (typically the primary `location` string). `RawData` carries the per-job raw bytes. `SourceURL` and `JobURL` both = `jobUrl`.

### Task 7 — Greenhouse parity update

Extend `internal/ats/greenhouse.go` to populate `LocationTexts` as a single-element `[]string` from `location.name` (nil if empty); `location_text` continues to be set from `location.name` (unchanged). Update the `ghJob` block comment to mention the new field. Migrate the HTTP path onto the shared helper from Task 4. Rename the existing `New` constructor to `NewGreenhouse`; update the call site in `cmd/fetcher/main.go`.

### Task 8 — Fetcher dispatch wiring

Register `"lever"` and `"ashby"` in the `adapters` map in `cmd/fetcher/main.go`. No change to the `atsAdapter` interface — both new adapters satisfy it structurally. Map `domain.Posting.LocationTexts` into the regenerated `InsertPostingSnapshotParams` in `buildSnapshotParams`. Add `nullStringSlice` (or equivalent) helper if the sqlc-generated parameter type for `text[]` requires nil/non-nil bridging; if sqlc yields plain `[]string`, no helper is needed.

### Task 9 — Tests

Add per-adapter unit tests using `httptest.Server` and recorded fixtures in `internal/ats/testdata/lever/` and `internal/ats/testdata/ashby/`. Cover the cases enumerated in the acceptance criteria. Reuse the Greenhouse test pattern for HTTP-error cases. The body-cap test lives at the shared helper level (Task 4); per-adapter tests assert that the wrapped error reaches the caller with the adapter's subsystem prefix. In `cmd/fetcher`, add a `buildSnapshotParams` test asserting `LocationTexts` flows through in both nil and populated cases. Greenhouse tests gain one case asserting `LocationTexts` is populated from `location.name` and is nil when `location.name` is empty.

### Task 10 — Documentation update

Update `agent-context/lib/project.md` to name `job_postings.first_seen_at` as the load-bearing first-seen / repost-detection signal. One paragraph: states that source-reported timestamps refresh on repost on at least some ATS boards, so the durable signal is our own observation. Source timestamps remain useful per-snapshot for change detection. This is durable architecture; placement should be in the snapshot/storage section, not buried in adapter-specific notes.

## Sequencing

Task 1 first. Tasks 2 and 3 can proceed after 1. Task 4 is independent and can run in parallel with 1–3. Tasks 5, 6, 7 depend on 3 and 4. Task 8 depends on 2, 3, 5, 6. Task 9 depends on 5, 6, 7, 8. Task 10 is independent and can ship any time.

Recommended order: 1, 2, 3, 4 (parallel where possible), then 7 (parity migration of Greenhouse exercises the helper before two new adapters lean on it), then 5 and 6 (parallel), then 8, then 9, then 10.

## Open questions

- **`location_texts` element format for Ashby `secondaryLocations`.** The `postalAddress` object has `addressLocality`, `addressRegion`, `addressCountry`. The chosen render is comma-separated, skipping empties. If real fixtures show this produces noisy strings (e.g. `", , US"` for sparsely-populated addresses), revisit before tests land.
- **Lever rate limits.** The v0 public API does not document a rate limit. The shared HTTP helper does not retry on 429. If field tests reveal 429 responses under our small watchlist load, retry/backoff is a separate spec.
