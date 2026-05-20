# Workable Adapter

## Goal

Add a Workable ATS adapter to the fetcher, enabling job postings from companies
hosted on Workable (`apply.workable.com/<slug>`) to be ingested and snapshotted
alongside Greenhouse, Lever, Ashby, and Workday boards. Workable surfaced as a
fifth common ATS while browsing GeekWire 200 startups.

## Scope

### In scope

- `internal/ats/workable.go`: Workable adapter (`Workable` struct, `FetchPostings` method).
- `internal/ats/workable_test.go`: adapter tests against `httptest.Server`.
- `internal/ats/testdata/workable/`: synthetic fixture JSON for tests (recorded
  real responses can replace these once a real Workable customer is seeded).
- `cmd/fetcher/main.go`: register `"workable"` in the adapters map.
- `internal/ats/greenhouse.go`: extend the `ats` package doc comment.
- `agent-context/lib/project.md`: add Workable to the ATS targets section.
- `agent-context/lib/watchlist.md`: supported-ATS list and probe URL pattern.

### Out of scope (v2 candidates)

- Per-posting description fetch. The widget endpoint returns summary-level
  fields only; full descriptions require either the authenticated
  `/spi/v3/jobs/{shortcode}` endpoint or an HTML scrape of the apply page. v1
  leaves `DescriptionText` nil. **Workable postings will not classify until
  this follow-up ships** (same trade-off `project.md` documents for Workday).
- Structured compensation. Not exposed in the widget response.
- Pagination. The widget endpoint is single-shot for current Workable boards.
  The adapter logs a warn if a response approaches a sanity ceiling so we
  notice if Workable changes that behavior.

## Acceptance criteria

- [x] `go build ./...` passes with the adapter added.
- [x] `go test ./internal/ats/...` passes, including new Workable tests.
- [x] `go vet ./...` reports no new issues.
- [x] A Workable company in the DB (`ats='workable'`, valid `board_token`) is
      fetched and snapshotted by `go run ./cmd/fetcher` without error.
- [x] The resulting `posting_snapshots` row has `title`, `location_text`,
      `job_url`, and `posted_at` populated; `description_text` is NULL
      (accepted for v1).
- [x] An invalid board_token (empty, contains `/`, `?`, `#`, or `://`)
      produces a wrapped `workable:` error and skips that company; no HTTP
      request fires.

## Wire shape (widget endpoint)

`GET https://apply.workable.com/api/v1/widget/accounts/{slug}` returns:

```json
{
  "name": "...",
  "description": "...",
  "jobs": [
    {
      "title": "...",
      "shortcode": "ABC123",
      "url": "https://apply.workable.com/<slug>/j/ABC123/",
      "application_url": "https://apply.workable.com/<slug>/j/ABC123/apply/",
      "department": "...",
      "employment_type": "Full-time",
      "published_on": "2024-04-25",
      "created_at": "2024-04-25T10:30:00Z",
      "country": "...", "city": "...", "state": "..."
    }
  ]
}
```

## Boundary inventory

| Name | Go struct field | JSON key | Notes |
|---|---|---|---|
| Jobs array | `wkResponse.Jobs` | `"jobs"` | `[]json.RawMessage` so RawData survives |
| Stable identifier | `wkJob.Shortcode` | `"shortcode"` | → `SourceID`; required-field anchor |
| Canonical URL | `wkJob.URL` | `"url"` | → `SourceURL` / `JobURL`; falls back to `application_url` |
| Apply URL | `wkJob.ApplicationURL` | `"application_url"` | Fallback only |
| Title | `wkJob.Title` | `"title"` | → `Posting.Title` |
| Department | `wkJob.Department` | `"department"` | → `Posting.Department` |
| Employment type | `wkJob.EmploymentType` | `"employment_type"` | → `Posting.EmploymentType` |
| Posted date | `wkJob.PublishedOn` | `"published_on"` | YYYY-MM-DD → `PostedAt` (midnight UTC) |
| First published | `wkJob.CreatedAt` | `"created_at"` | RFC3339 → `SourceFirstPublishedAt` |
| Location | `wkJob.City` / `State` / `Country` | flat fields | Joined with `", "`, nil if all empty |

## Open questions

1. **Real-fixture confirmation**: v1 fixtures are synthetic, built from
   Workable's documented widget response shape. Field names need confirmation
   against a real response (`curl https://apply.workable.com/api/v1/widget/accounts/<real-slug>`)
   before the first production fetch. If any field name differs, update
   `workable.go` wire tags and replace the fixtures.

2. **Seed company**: the user found a Workable-hosted company in the GeekWire
   200 but hasn't named which one. Adapter ships without a seed row; add one
   to `internal/db/seeds/companies.sql` once the company is identified.

3. **Description follow-up**: same architectural choice as Workday — raise the
   per-company timeout and fetch per-posting, or move description ingestion to
   a post-snapshot enrichment step.
