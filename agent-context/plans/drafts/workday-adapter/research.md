# Workday Adapter Research

Research notes from web search (2026-05). Not part of the spec; captured here for reference.

## API Endpoint

Workday exposes a CXS JSON API (not the HTML careers page) at:

```
POST https://{tenant}.wd{N}.myworkdayjobs.com/wday/cxs/{tenant}/{site}/jobs
```

No auth required for public boards. Request body:

```json
{"appliedFacets": {}, "limit": 20, "offset": 0, "searchText": ""}
```

Headers: `Content-Type: application/json`, `Accept: application/json`, standard User-Agent.

## Response Shape (listings)

```json
{
  "total": 150,
  "jobPostings": [
    {
      "title": "Software Engineer",
      "externalPath": "jobs/R-123456/Software-Engineer",
      "locationsText": "San Francisco, CA",
      "postedOn": "2025-05-10",
      "bulletFields": ["REQ123"],
      "externalUrl": "https://nvidia.wd5.myworkdayjobs.com/en-US/NVIDIAExternalCareerSite/job/US-CA-Santa-Clara/..."
    }
  ]
}
```

Descriptions are NOT included in the listings response. Full descriptions require a separate GET:

```
GET /wday/cxs/{tenant}/{site}/job/{externalPath}
```

## URL Structure

- **Tenant**: first segment of subdomain (`nvidia` from `nvidia.wd5.myworkdayjobs.com`)
- **wd{N}**: data center number — varies per company (wd1, wd3, wd5, wd12, etc.)
- **Site**: job board path segment (e.g., `NVIDIAExternalCareerSite`)

A company's Workday careers page URL exposes all three:
`https://{tenant}.wd{N}.myworkdayjobs.com/en-US/{site}/...`

The locale prefix (`en-US`) is NOT part of the CXS API path.

## Pagination

Offset-based: increment `offset` by `limit` per request. Terminate when:
- `jobPostings` array is empty, OR
- `offset + len(jobPostings) >= total`

## Rate Limiting / Anti-Scraping

- HTTP 429 on rate limit
- CSRF tokens exist but not required for server-side POST requests
- Session cookie (`wday_vps_cookie`) needed by some tenants (minority)
- Most public boards work with a standard User-Agent and no cookies

## postedOn Date Format

Reported as `"YYYY-MM-DD"` in most implementations. Some boards may return a relative
string (`"Posted 5 Days Ago"`). Adapters must guard against non-date formats.

## Board Token Concept

Workday has no equivalent to Greenhouse's `board_token`. The routing identifier is
tenant + wd{N} host + site — three components. Must be encoded together for the
`board_token` DB column.

## No Structured Compensation

Listings endpoint does not expose structured pay data.

## Sources

- jobo.world/ats/workday
- apify.com/gooyer.co/myworkdayjobs/api
- dev.to/hasdata_com/building-a-production-ready-job-board-scraper-with-python-pgd
- github.com/chuchro3/WebCrawler
