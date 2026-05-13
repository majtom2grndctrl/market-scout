# Company Watchlist

> **Read this when:** adding, removing, or sourcing companies for the scrape run, or evaluating ATS coverage.
> **Key invariant:** The seed file is the canonical source of truth. Seeded fields: `name`, `ats`, `board_token`, `industry`.
> **Related:** `project.md` §ATS targets, `index.md`, `internal/ats/`

---

## New companies

### What to capture

Required fields:

| Field | Description |
|---|---|
| `name` | Display name |
| `ats` | One of: `greenhouse`, `lever`, `ashby` |
| `board_token` | Company's slug on that ATS |

`careers_page_url` is optional. Capture it when visible — it strengthens dedup.

### Non-goals

- Does not track unverified leads or a candidates pipeline
- Companies on unsupported ATS platforms (Workday, Rippling, Jobvite, etc.) are discarded
- Companies without a valid board token on a supported ATS are discarded

### Dedup

Before visiting any careers pages, query the `companies` table for the candidate names first. Normalize both sides: strip punctuation and whitespace, lowercase, compare for equality. Substring matches are not duplicates.

Return enough context to disambiguate: name, ATS, board token, industry, and careers page URL. Names collide across unrelated companies. Disambiguate using ATS board URL plus industry — the board URL is the strongest signal.

Flag uncertain matches; never silently skip or crawl. Drop confirmed duplicates.

### Board token verification

Probe the ATS endpoint directly:

| ATS | URL pattern |
|---|---|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/<token>/jobs` |
| Ashby | `https://jobs.ashbyhq.com/<token>` |
| Lever | `https://api.lever.co/v0/postings/<token>?mode=json` |

Success signals:

| ATS | Success signal |
|---|---|
| Greenhouse | HTTP 200; `jobs` array present (empty array = valid board, no openings — flag, don't discard) |
| Ashby | HTTP 200; page renders job board (not Ashby's "board not found" page) |
| Lever | HTTP 200; JSON array returned (empty array = valid board, no openings — flag, don't discard) |

### Confirmed results

Verified companies go directly into the seed file.
