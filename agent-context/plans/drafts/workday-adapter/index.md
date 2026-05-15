# Workday Adapter

## Goal

Add a Workday ATS adapter to the fetcher, enabling job postings from Workday-hosted careers
pages to be ingested, snapshotted, and classified alongside Greenhouse, Lever, and Ashby boards.
Workday is one of the most common enterprise ATS platforms; several target companies in the
watchlist are inaccessible without it.

## Scope

### In scope

- `internal/ats/workday.go`: Workday adapter (`Workday` struct, `FetchPostings` method)
- `internal/ats/httpfetch.go`: add `httpPost` helper (POST with JSON body; parallel to `httpFetch`)
- `internal/ats/workday_test.go`: adapter tests against `httptest.Server`
- `internal/ats/testdata/workday/`: recorded fixture JSON for tests
- `cmd/fetcher/main.go`: register `"workday"` in the adapters map
- `agent-context/lib/watchlist.md`: update supported ATS list; document board_token encoding
- `agent-context/lib/project.md`: add Workday to ATS targets section

### Out of scope

- Per-job description fetch. The listings endpoint does not include descriptions; fetching them
  requires one GET per posting. With sequential fetching and a 45s per-company budget, large
  boards (100+ jobs) would reliably time out. `DescriptionText` stays nil in v1. Follow-up
  required before Workday postings can be classified.
- Structured compensation. The listings endpoint exposes no pay data.
- Session/CSRF pre-flight. Most public Workday boards accept POST requests without a session
  cookie. Companies that require it are treated as fetch errors.

## Acceptance criteria

- [ ] `go build ./...` passes with the adapter added.
- [ ] `go test ./internal/ats/...` passes, including new Workday tests.
- [ ] `go vet ./...` and `staticcheck ./...` report no new issues.
- [ ] A Workday company in the DB (e.g., `ats='workday'`, valid `board_token`) is fetched and
      snapshotted by `go run ./cmd/fetcher` without error.
- [ ] Snapshots for Workday companies have populated `title`, `source_id`, `source_url`,
      `job_url`, `location_text`, and `posted_at` columns (where the board exposes them).
      `description_text` is NULL — accepted for v1.
- [ ] Companies with `ats='workday'` but an unparseable board_token produce a structured error
      and skip that company (no crash, other companies proceed).
- [ ] Unknown `ats` values (already-existing behavior) continue to emit a warn and skip.
- [ ] Pagination: a board with more jobs than the page limit is fully fetched across multiple
      requests.
- [ ] The sanity ceiling on pagination triggers a structured error (not an infinite loop) for a
      server that never returns a short page.

## Tasks

### Task 1: `httpPost` helper

Add `httpPost(ctx, client, url, body []byte) ([]byte, error)` to
`internal/ats/httpfetch.go`. Issues a POST with `Content-Type: application/json` and
`Accept: application/json` headers. Same response validation and body-size cap as `httpFetch`.
Same User-Agent header (`market-scout/0.1 ...`).

### Task 2: Workday adapter

Create `internal/ats/workday.go`.

**Struct and constructor:**

```go
// Proposed design — remove after implementation
type Workday struct {
    client  *http.Client
    baseURL string // override for tests; production callers use workdayBaseURL
}

func NewWorkday(client *http.Client) *Workday { ... }
func newWorkdayWithBaseURL(client *http.Client, baseURL string) *Workday { ... }
```

`newWorkdayWithBaseURL` exists for test isolation (same pattern as Greenhouse, Lever, Ashby).

**board_token encoding:**

The `board_token` DB column encodes the full Workday routing identifier as:
`{host}/{site}` — e.g., `nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite`.

The adapter parses this on every `FetchPostings` call:
- Split on `/` (first occurrence only) → `host` and `site`.
- Extract tenant from host: first segment before the first `.`
  (e.g., `nvidia` from `nvidia.wd5.myworkdayjobs.com`).
- Construct API URL: `https://{host}/wday/cxs/{tenant}/{site}/jobs`

A board_token that does not contain `/` or yields an empty host/site/tenant is a structured
error; the fetcher skips that company and logs a `[workday] invalid board_token` warn.

**Test-only base URL override:**

`newWorkdayWithBaseURL` accepts a base URL (e.g. `http://127.0.0.1:PORT`) used instead of
the `https://{host}` prefix. This lets tests target `httptest.Server` without real Workday
hostnames. The tenant and site are still parsed from `boardToken`; only the scheme+host is
overridden.

**Wire types:**

```go
// Proposed design — remove after implementation
type wdResponse struct {
    Total       int               `json:"total"`
    JobPostings []json.RawMessage `json:"jobPostings"`
}

type wdJob struct {
    Title         string `json:"title"`
    ExternalPath  string `json:"externalPath"`
    LocationsText string `json:"locationsText"`
    PostedOn      string `json:"postedOn"`
    ExternalUrl   string `json:"externalUrl"`
    // BulletFields is an array; the first element is typically the requisition ID.
    // Not used for SourceID — externalPath is the stable identifier.
    BulletFields []string `json:"bulletFields"`
}
```

Per-job raw bytes are preserved via `json.RawMessage` in `wdResponse.JobPostings` so `RawData`
carries the original listing payload.

**SourceID:** `externalPath` (unique per posting, stable across fetches).

**SourceURL:** `externalUrl` when present; constructed fallback
`https://{host}/en-US/{site}/{externalPath}` when `externalUrl` is absent.

**JobURL:** same as `SourceURL`.

**Pagination:** limit 20 (Workday's default). Increment `offset` by `limit` per page.
Terminate when `len(jobPostings) == 0` OR `offset + len(rawJobs) >= total`.
Sanity ceiling: 1000 pages (matches Lever's ceiling); warn at 500 (same pattern).

**`postedOn` parsing:** `postedOn` is a `YYYY-MM-DD` date string. Parse with
`time.Parse("2006-01-02", job.PostedOn)`, treat as midnight UTC. Populates both `PostedAt`
and `SourceFirstPublishedAt`. Non-parseable `postedOn` strings (e.g. `"Posted 5 Days Ago"`)
emit a `[workday] warn` and leave both fields nil — do not abort the fetch.

**`SourceLastModifiedAt`:** always nil (Workday listings expose no last-modified timestamp).

**LocationText / LocationTexts:** `locationsText` is a single string (may contain multiple
locations separated by `"; "` or `", "`). Wrap verbatim in a one-element `LocationTexts`
slice. Do not split — the delimiter is not stable across boards.

**DescriptionText:** nil (v1 deferred).

**Compensation:** nil (not exposed in listings).

**Error prefix:** `[workday]` for all adapter log lines. Error strings: `workday: ...` prefix.

### Task 3: Workday tests

Create `internal/ats/workday_test.go` and fixture files in
`internal/ats/testdata/workday/`.

Required tests:
- `TestWorkdayAdapter_SuccessfulParse` — single-page response; verify `SourceID`,
  `SourceURL`, `JobURL`, `Title`, `LocationText`, `LocationTexts`, `PostedAt`,
  `SourceFirstPublishedAt`, nil `DescriptionText`, nil `CompensationMin`.
- `TestWorkdayAdapter_PaginatesUntilShortPage` — multi-page (page 1 full, page 2 short);
  verify total posting count and that the adapter issued two POST requests.
- `TestWorkdayAdapter_StopsWhenOffsetExceedsTotal` — `total` says 25, one page of 20, second
  page terminates because `offset + 20 >= total` (20 + 20 >= 25 but page 2 only has 5 jobs).
- `TestWorkdayAdapter_InvalidBoardToken` — board_token without `/`; expect non-nil error.
- `TestWorkdayAdapter_HTTPError` — server returns 500; expect wrapped error, no postings.
- `TestWorkdayAdapter_MissingExternalPath` — a job entry with empty `externalPath`; expect
  structured error naming the index.

Fixture files record real Workday CXS response JSON. Follow the same
`loadAdapterFixture(t, "workday", "jobs_page1.json")` pattern used in other adapter tests.

### Task 4: Register adapter + update docs

In `cmd/fetcher/main.go`, add `"workday": ats.NewWorkday(httpClient)` to the `adapters` map.

In `agent-context/lib/watchlist.md`:
- Remove `Workday` from the "unsupported platforms" list.
- Add it to the supported ATS table with its board_token format.
- Add a probe URL pattern for verification.

In `agent-context/lib/project.md`, update the ATS targets section to include Workday.

## Sequencing

**Phase 1 (sequential):** Task 1 — `httpPost` helper. Unblocks Task 2 (adapter uses it).

**Phase 2 (concurrent):** Task 2 (adapter), Task 3 (tests + fixtures) — adapter and tests
can be drafted together; tests reference the adapter's exported constructor.

**Phase 3 (sequential):** Task 4 — registration and doc updates. Depends on Task 2 (adapter
type must exist to reference in main.go).

## Boundary inventory

| Name | Go struct field | JSON key | Notes |
|---|---|---|---|
| Total count | `wdResponse.Total` | `"total"` | Number of matching jobs |
| Jobs array | `wdResponse.JobPostings` | `"jobPostings"` | Array of raw job objects |
| Job ID/path | `wdJob.ExternalPath` | `"externalPath"` | Used as SourceID |
| Job URL | `wdJob.ExternalUrl` | `"externalUrl"` | Used as SourceURL and JobURL |
| Title | `wdJob.Title` | `"title"` | → `domain.Posting.Title` |
| Location | `wdJob.LocationsText` | `"locationsText"` | Single string; wrapped in 1-element LocationTexts |
| Posted date | `wdJob.PostedOn` | `"postedOn"` | YYYY-MM-DD string → PostedAt, SourceFirstPublishedAt |
| Req ID | `wdJob.BulletFields` | `"bulletFields"` | Captured in RawData; not used as SourceID |

## Open questions

1. **`postedOn` reliability**: Research confirms YYYY-MM-DD for most boards, but some may
   return relative strings (`"Posted N Days Ago"`). The adapter warns and skips the date
   rather than aborting. Verify against real boards before shipping.

2. **Description fetch follow-up**: Should the follow-up ticket raise the per-company timeout
   for Workday companies, or should description fetching be moved to a post-snapshot
   enrichment step (similar to how `batch-enrich` handles classification)? The enrichment-step
   approach would require a DB-visible signal that Workday postings lack descriptions so they
   can be retried. Decision not needed for v1 but should be resolved before the follow-up
   ticket is written.

3. **Session cookie requirement**: A minority of Workday tenants require the `wday_vps_cookie`
   session cookie. The v1 adapter ignores this; those companies produce a fetch error.
   Worth noting when seeding Workday companies — probe the CXS endpoint directly and confirm
   a 200 response before adding to the watchlist.

4. **`locationsText` delimiter**: If a multi-location role appears, `locationsText` may be
   something like `"New York, NY; San Francisco, CA"` or `"New York, NY | San Francisco, CA"`.
   v1 wraps the whole string without splitting. If multi-location roles are common, a
   follow-up can add delimiter detection.
