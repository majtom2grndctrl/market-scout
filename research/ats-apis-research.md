# ATS API Research

> **Purpose:** Inform a normalized Postgres schema that isn't over-fitted to any single ATS.
> **Sources:** Live API docs and live API responses fetched May 2026.

---

## Greenhouse

**Public board base URL:** `https://boards-api.greenhouse.io/v1/boards/{board_token}/jobs`  
**Auth required for reads:** No — all GET endpoints are public.  
**Pagination:** Not formally paginated at the list level (returns all open jobs).

### Job identifiers

| Field | Type | Notes |
|---|---|---|
| `id` | integer | Job *post* ID — used for applications |
| `internal_job_id` | integer | The underlying job ID (null for prospect posts) |

Two IDs exist because Greenhouse allows multiple job posts per job (e.g., for different offices). `internal_job_id` is the stable parent; `id` is posting-specific.

### Fields returned (basic response)

| Field | Type | Notes |
|---|---|---|
| `id` | integer | Job post ID |
| `internal_job_id` | integer | Parent job ID |
| `title` | string | Job title |
| `updated_at` | ISO 8601 | Last update time |
| `requisition_id` | string | Internal req number |
| `location` | `{name: string}` | Location as an object with a display name |
| `absolute_url` | string | URL to careers page posting |
| `language` | string | Language code (e.g. "en") |
| `metadata` | object/null | Custom fields exposed by the company |

### Additional fields with `?content=true`

- `content` — HTML job description
- `departments` — array of `{id, name, parent_id, child_ids}`
- `offices` — array of `{id, name, location, parent_id, child_ids}`

### Notable gaps

- No `posted_at` / `created_at` in the standard response
- No employment type (full-time, contract, etc.)
- No remote/workplace type field
- Department is only returned with `?content=true`, and as a structured object (not a plain string)

---

## Lever

**Public board base URL:** `https://api.lever.co/v0/postings/{company-slug}`  
**Auth required for reads:** No — the v0 API is public for GET requests.  
**Pagination:** `limit` (1–100) + opaque `offset` token; response includes `next` and `hasNext`.

### Job identifiers

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | Single stable identifier per posting |

### Fields returned

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | Posting ID |
| `text` | string | **Job title** (not `title`) |
| `createdAt` | integer | Milliseconds since epoch (not ISO 8601) |
| `country` | string | Country code |
| `categories` | object | See below |
| `workplaceType` | string | `"hybrid"`, `"remote"`, `"onsite"`, `"unspecified"` |
| `description` | string | HTML description |
| `descriptionPlain` | string | Plain text description |
| `hostedUrl` | string | URL to job posting |
| `applyUrl` | string | Application URL |
| `salaryRange` | object | `{min, max, currency, interval}` — may be absent |

### `categories` object fields

| Field | Notes |
|---|---|
| `commitment` | Employment type as free text (e.g. `"Full-time"`, `"Contract"`) |
| `department` | Department name string |
| `location` | Location string |
| `team` | Team name string |
| `allLocations` | Array of location strings |

### Notable quirks

- Title is `text`, not `title` — adapters must remap
- `createdAt` is milliseconds (not ISO 8601) — requires conversion
- Employment type lives inside `categories.commitment` as free text, not an enum
- `workplaceType` values use lowercase (`"remote"`) vs. Ashby's PascalCase (`"Remote"`)

---

## Ashby

**Public board base URL:** `https://api.ashbyhq.com/posting-api/job-board/{clientname}`  
**Auth required for reads:** No — the `posting-api` endpoint is public. (The separate `jobPosting.list` endpoint in the partner API requires auth.)  
**Pagination:** None at the board level — returns all jobs in a `{ jobs: [...] }` wrapper object.

### Job identifiers

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | Single stable identifier per posting |

### Fields returned

| Field | Type | Notes |
|---|---|---|
| `id` | UUID string | Posting ID |
| `title` | string | Job title |
| `department` | string | Department name |
| `team` | string | Team name |
| `employmentType` | enum string | `"FullTime"`, `"PartTime"`, `"Intern"`, `"Contract"`, `"Temporary"` |
| `workplaceType` | enum string | `"OnSite"`, `"Remote"`, `"Hybrid"` |
| `isRemote` | boolean | Remote indicator (redundant with `workplaceType`) |
| `isListed` | boolean | Whether visible on public board |
| `location` | string | Primary location as display string |
| `address` | object | Structured: `postalAddress.{addressLocality, addressRegion, addressCountry}` |
| `secondaryLocations` | array | Additional locations with same structure as `address` |
| `publishedAt` | ISO 8601 | Publication timestamp |
| `descriptionHtml` | string | HTML description |
| `descriptionPlain` | string | Plain text description |
| `jobUrl` | string | Ashby-hosted job page |
| `applyUrl` | string | Application URL |

Optional with `?includeCompensation=true`: compensation tiers and component breakdowns.

### Notable observations

- Most complete and consistent API of the three
- Both `isRemote` (bool) and `workplaceType` (enum) present — `isRemote` is redundant but useful as a simple flag
- `publishedAt` is proper ISO 8601; no millisecond-epoch quirks
- Location dual-representation: plain string + structured address object

---

## Cross-ATS Comparison

| Concept | Greenhouse | Lever | Ashby |
|---|---|---|---|
| **Job ID type** | integer | UUID string | UUID string |
| **Title field name** | `title` | `text` | `title` |
| **Location format** | `location.name` (string) | `categories.location` (string) | `location` (string) + `address` (object) |
| **Department** | object array (content=true) | `categories.department` (string) | `department` (string) |
| **Team** | — | `categories.team` (string) | `team` (string) |
| **Employment type** | — | `categories.commitment` (free text) | `employmentType` (enum) |
| **Workplace/remote** | — | `workplaceType` (lowercase enum) | `workplaceType` (PascalCase enum) + `isRemote` |
| **Posted date** | — | `createdAt` (ms epoch) | `publishedAt` (ISO 8601) |
| **Updated date** | `updated_at` (ISO 8601) | — | — |
| **Job URL field name** | `absolute_url` | `hostedUrl` | `jobUrl` |
| **Apply URL** | — | `applyUrl` | `applyUrl` |
| **Auth for public reads** | No | No (v0) | No (posting-api) |
| **Pagination** | None (all jobs) | limit + offset token | None (all jobs) |
| **Response wrapper** | `{jobs: [...]}` | array | `{jobs: [...]}` |

### Fields that look common but have incompatible formats

- **Employment type:** Lever returns free text (`"Full-time"`, `"Part time"`) while Ashby returns a consistent enum (`"FullTime"`). Greenhouse doesn't return it at all. Normalizing requires a mapping layer.
- **Workplace type:** Lever uses lowercase (`"remote"`) and Ashby uses PascalCase (`"Remote"`). Same concepts, different casing.
- **Dates:** Lever `createdAt` is milliseconds since epoch; Ashby `publishedAt` is ISO 8601; Greenhouse `updated_at` is ISO 8601 but means something different (last modified, not posted).
- **Location:** All three return a primary location string, but the field path differs in each. Ashby additionally returns structured address data; the others don't.
- **Job ID:** Greenhouse uses integers; Lever and Ashby use UUIDs. Must normalize to string for a shared column.

---

## Schema Implications

### Normalize into columns

These fields are consistently available (or inferrable) across all three ATS and are the most query-relevant for trend analysis:

| Column | Type | Source mapping |
|---|---|---|
| `ats_name` | `text` | `'greenhouse'` / `'lever'` / `'ashby'` |
| `ats_job_id` | `text` | Greenhouse: `id::text`, Lever: `id`, Ashby: `id` |
| `ats_internal_job_id` | `text` / null | Greenhouse only: `internal_job_id::text` |
| `title` | `text` | Greenhouse: `title`, Lever: `text`, Ashby: `title` |
| `location_text` | `text` / null | Greenhouse: `location.name`, Lever: `categories.location`, Ashby: `location` |
| `department` | `text` / null | Greenhouse: from departments array, Lever: `categories.department`, Ashby: `department` |
| `team` | `text` / null | Lever: `categories.team`, Ashby: `team` |
| `employment_type` | `text` / null | Lever: `categories.commitment` (normalized), Ashby: `employmentType` (normalized) |
| `workplace_type` | `text` / null | Lever + Ashby: normalize to `remote` / `hybrid` / `onsite` |
| `is_remote` | `boolean` | Derived from `workplace_type = 'remote'` or Ashby `isRemote` |
| `posted_at` | `timestamptz` / null | Lever: convert from ms epoch, Ashby: `publishedAt`, Greenhouse: null |
| `job_url` | `text` / null | Greenhouse: `absolute_url`, Lever: `hostedUrl`, Ashby: `jobUrl` |
| `fetched_at` | `timestamptz` | Set by the fetcher at runtime — not from the API |

### Push into JSONB raw blob

Everything else belongs in a `raw_data jsonb` column on the snapshot row:

- Full API response (preserves fields added later without migrations)
- Greenhouse `offices`, `content`, `metadata`, `requisition_id`
- Lever `salaryRange`, `lists`, description HTML/plain, `descriptionBody`, `additional`
- Ashby `address`, `secondaryLocations`, `descriptionHtml`, compensation data, `isListed`
- Any field not in the normalized list above

**Rule of thumb:** if you'd filter, group, or trend-query by it, normalize it. Everything else goes in the blob.
