# Company Watchlist

> **Read this when:** adding, removing, or sourcing companies for the scrape run, or evaluating ATS coverage.
> **Key invariant:** The seed file is the canonical source of truth. Seeded fields: `name`, `ats`, `board_token`, `industry`.
> **Related:** `project.md` §ATS targets, `index.md`, `apps/tools/internal/ats/`

---

## New companies

Annotation is done by an agent or human; verification and seed-row emission are handled by `apps/tools/cmd/onboard` (see §Research file annotation).

### What to capture

Required fields:

| Field | Description |
|---|---|
| `name` | Display name |
| `ats` | One of: `greenhouse`, `lever`, `ashby`, `workday`, `workable` |
| `board_token` | Company's slug on that ATS (Workday uses `{host}/{site}`, e.g. `nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite`; see *Workday token discovery* below. Workable uses the lowercase slug from `apply.workable.com/<slug>`) |

`careers_page_url` is optional. Capture it when visible — it strengthens dedup.

### Non-goals

- Does not track unverified leads or a candidates pipeline
- Companies on unsupported ATS platforms (Rippling, Jobvite, etc.) are discarded
- Workday tenants that require a `wday_vps_cookie` session cookie fail with a fetch error in v1; only public Workday boards are supported
- Companies without a valid board token on a supported ATS are discarded
- Eligibility is ATS-based only — supported platform plus valid board token. Not filtered by company size, stage, or type. A niche watchlist emerges from what a user chooses to onboard, not from a built-in restriction.

### Dedup

Before browser work, call `dedup_candidates`. It checks four signals in priority order: exact `(ats, board_token)`, careers-page domain, exact normalized name, then fuzzy normalized name. Name normalization strips punctuation and whitespace and lowercases both sides. Substring matches are not exact-name matches.

Domain matching compares normalized hosts from candidate and stored careers-page URLs. An invalid candidate careers URL is treated as absent; token and name matching still run. Fuzzy matching is a last resort when exact name and domain find no match. It uses trigram similarity with a `0.4` threshold.

`known_unsupported` is an independent, informational field for browser triage. It reports a prior unsupported finding: name, nullable URL, nullable detected platform, reason, first-seen time, last-checked time, and whether the finding is stale. Lookup prefers a matching URL host over a matching name. It never changes `verdict`, `match_kind`, `matches`, or `match_count`.

For manual or sidecar review, retain enough context to disambiguate: name, ATS, board token, industry, and careers page URL. Names and domains can collide across unrelated companies. `dedup_candidates` returns matched company identity, `match_count`, `matches`, `match_kind`, and `reason`. When signals converge on one company, its strongest signal wins: `token > domain > name_only > fuzzy_name`.

**Skip rule.** A candidate is a duplicate when the DB already has a company with the same `(ats, board_token)` pair and a `posting_snapshots` row within the last 30 days. `(ats, board_token)` is the DB's unique constraint and the strongest dedup signal; name is used to surface matches for human inspection but is not part of the dedup tuple. Drop confirmed duplicates.

**Recency threshold.** "Recent postings" means at least one `posting_snapshots` row for any of the company's job postings with `fetched_at` within the last 30 days. A company with an `(ats, board_token)` match but no snapshot within 30 days is stale.

**Review-only matches.** Exact-name, domain, and fuzzy-name matches are stale even when the matched company has recent postings. They never silently produce a duplicate verdict. Use `match_kind` and `reason` to distinguish these review cases from token duplicates.

**Fuzzy result cap.** At most the top three fuzzy-name matches are returned, ordered by similarity. `match_count` preserves the true distinct-company count across all signals before that cap.

**Stale matches.** A match whose DB row has no recent postings, a non-matching ATS, or a domain that no longer resolves is *not* a dedup case — it is a merge problem. Flag with the `stale-needs-merge` status (see annotation section) for human review. Merging stale DB rows with fresh research is out of scope for the onboarding pass.

Flag uncertain matches; never silently skip or crawl.

### Unsupported-company registry

The registry records unsupported findings. Write an entry when `detect_ats` returns `unsupported-ats` or browser investigation concludes `no-careers`.

Each entry captures the company name, nullable URL, nullable detected platform, reason, first-seen time, and last-checked time. A finding is stale after 90 days without a check. `stale: true` means a re-check may be worthwhile; it never requires one.

Trust tiers differ by reason. `unsupported_ats` is Deterministic: `detect_ats` derives it from URL parsing. `no_careers` is Agent-asserted: the live path has no tool observation. Treat `no_careers` as prior browser judgment, not verified fact.

### Board token verification

Probe the ATS endpoint directly:

| ATS | URL pattern |
|---|---|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/<token>/jobs` |
| Ashby | `https://jobs.ashbyhq.com/<token>` |
| Lever | `https://api.lever.co/v0/postings/<token>?mode=json` |
| Workday | `https://{host}/wday/cxs/{tenant}/{site}/jobs` (POST, no auth for public boards; `{tenant}` is the first DNS label of `{host}`) |
| Workable | `https://apply.workable.com/api/v1/widget/accounts/<token>` (GET, no auth) |

Success signals:

| ATS | Success signal |
|---|---|
| Greenhouse | HTTP 200; `jobs` array present (empty array = valid board, no openings — flag, don't discard) |
| Ashby | HTTP 200; page renders job board (not Ashby's "board not found" page) |
| Lever | HTTP 200; JSON array returned (empty array = valid board, no openings — flag, don't discard) |
| Workday | HTTP 200; `jobPostings` array present (empty array = valid board, no openings — flag, don't discard). Tenants gated behind a `wday_vps_cookie` session cookie return an error and are unsupported in v1. |
| Workable | HTTP 200; `jobs` array present (empty array = valid board, no openings — flag, don't discard). Workable v1 omits descriptions, so postings won't classify until the per-posting description fetch ships. |

### ATS detection

Map a careers-page URL to an `ats` value by matching the host and path against these patterns. First match wins. Patterns capture the `board_token` portion in `<…>`.

| URL pattern | ATS | Captured `board_token` |
|---|---|---|
| `boards.greenhouse.io/<token>` | `greenhouse` | `<token>` |
| `job-boards.greenhouse.io/<token>` | `greenhouse` | `<token>` |
| `jobs.lever.co/<token>` | `lever` | `<token>` |
| `jobs.ashbyhq.com/<token>` | `ashby` | `<token>` |
| `<host>.myworkdayjobs.com/<locale>/<site>` | `workday` | `<host>.myworkdayjobs.com/<site>` (see *Workday token discovery*) |
| `apply.workable.com/<token>` | `workable` | `<token>` (lowercase) |

A careers page that does not match any pattern is `unsupported-ats`. A careers page that matches a pattern but whose probe (per *Board token verification*) fails is `invalid-token` — the ATS is supported but the slug or tenant is wrong.

### Workday token discovery

Workday site names are not derivable from the company name. Visit the company's careers page and locate the Workday URL. The URL contains a locale segment between host and site: `https://{host}/{locale}/{site}/jobs`. The locale matches `[a-z]{2}(-[A-Z]{2})?` (e.g. `en-US`, `de`, `fr-FR`). Extract `host` and `site`; strip the locale. The board token is `{host}/{site}` (e.g. `nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite`).

### Detection is a shared boundary

URL-pattern detection is one source of truth. The batch research sidecar and the live MCP onboarding path apply the same URL rules — neither keeps its own copy, so a rule change lands once.

Detection and validation are deterministic and side-effect-free: no network, no DB. Verification is a separate, live step. The live MCP preflight hands off to `add_company`, which probes the ATS and writes the company row. The batch sidecar hands off to `cmd/onboard`, which verifies and writes seed rows.

### Live discover-and-onboard

Use the live MCP path for one-off additions. Use the sidecar workflow and `cmd/onboard` for batch verification and seed-file writes.

Agent or human finds the company homepage and careers page. Agent may inspect browser redirects, page links, scripts, and network request URLs. Pass the careers URL and relevant observed URLs to `detect_ats`.

`detect_ats` parses supplied URL evidence only. It does not drive a browser, crawl the web, probe ATS endpoints, or write to the DB.

Map the selected `detect_ats` result into `add_company`:

| `detect_ats` selected field | `add_company` input |
|---|---|
| `ats` | `ats` |
| `board_token` | `board_token` |

Then call `add_company` with the selected `ats` and `board_token`, plus the company details. `add_company` probe success is the falsifiability gate. Failed probes insert nothing.

Seed-file drift remains visible through the existing `add_company` follow-up.

### Browser-led discovery run

Use this path when one source yields many company leads and browser time should start after dedup.

Loop:

1. Discover candidates from a source: list page, article, manual notes, or pasted research.
2. Call `dedup_candidates` with each candidate name and any known `ats` / `board_token`.
3. Drop `duplicate` results before browser work. Set aside all `stale` results for merge or disambiguation review.
4. Investigate new survivors in the browser. Collect homepage, careers, redirect, and ATS URLs.
5. Pass observed URL evidence to `detect_ats`.
6. Pass the selected ATS result to `add_company`.

`add_company` stays the onboarding gate. Its ATS probe must pass before a company enters the fetcher set. Seed-file drift is still surfaced by the existing `add_company` follow-up.

Sourcing paths: batch sidecar verifies research lists and writes seed rows; one-off live onboard adds one known company; browser-led preflight dedups many leads before browser investigation.

### Research file annotation

Bulk sourcing from research lists (e.g. `research/geekwire-200.md`, a YC batch page, a regional startup roundup) uses a structured JSONL sidecar. The source — wherever it lives — is not modified by this workflow. Annotation lives in `research/<source-stem>.jsonl`, one JSON object per line.

**Sidecar generation.** The sidecar is produced by an agent reading the source: a local markdown file, a web page open in a browser-sidebar agent, copy-pasted text, or any other research surface. The agent emits one JSON record per company in a fenced code block conforming to the schema below. The output is pasted verbatim into `research/<source-stem>.jsonl`. No dedicated tool generates sidecars; the schema is the contract that makes agent output deterministic. Once generated, the sidecar is the only file the verification tool reads or writes.

**Three field layers** per record:

| Layer | Writer | Examples |
|---|---|---|
| Source | Annotating agent, once at sidecar generation. Immutable thereafter. | `rank`, `name`, source-row metadata (industry, location, headcount, etc.). |
| Annotation | Annotating agent, any pass. Mutable until terminal. | `url`, `careers_url`, `ats`, `board_token`, `notes`, annotator-set `status`. |
| Tool-stamped | Verification tool only. | `verified_at`, `verified_run_id`, tool-set `status`. |

**Falsifiability gate.** Only the verification tool sets `verified_at` and `verified_run_id`. A model or human annotator cannot self-attest verification. The run ID enables provenance tracing across re-runs.

**Status taxonomy.**

| Value | Meaning | Set by |
|---|---|---|
| `unsupported-ats` | Careers page uses an ATS not in *What to capture*. | Annotator or tool. |
| `no-careers` | No careers page or no open roles. | Annotator or tool. |
| `dead` | Company defunct or acquired. | Annotator only. |
| `duplicate` | Already in the seed file with a working ATS and recent postings (see *Dedup*). | Annotator or tool. |
| `stale-needs-merge` | Exact-name, domain, or fuzzy-name match needs human merge review. | Tool. |
| `invalid-token` | Supported ATS detected, but the ATS probe failed. | Tool. |

Statuses are terminal. A record is in progress iff it has neither a `status` nor a `verified_at`.

**Sidecar record schema.** One JSON object per line. The file's name mirrors the source: `research/<source-stem>.jsonl`.

| Field | Type | Layer | Nullable | Notes |
|---|---|---|---|---|
| `rank` | int | source | no | 1-based position in source list. Primary key within file. |
| `name` | string | source | no | Display name as it appears in the source. |
| `source` | object | source | no | Immutable sub-object: `industry` (string\|null), `location` (string\|null), `year_founded` (int\|null), `employees` (int\|null), `employee_change_pct` (int\|null). Verbatim from the source row. |
| `url` | string | annotation | yes | Company homepage URL. |
| `careers_url` | string | annotation | yes | Direct URL to careers page. Required when not `no-careers`. |
| `ats` | enum string | annotation | yes | One of `greenhouse`, `lever`, `ashby`, `workday`, `workable`. |
| `board_token` | string | annotation | yes | Format per *Board token verification* above. |
| `notes` | string | annotation | yes | Free text. Human disambiguation, observations. |
| `status` | enum string | annotation or tool | yes | See status taxonomy above. Mutually exclusive with `verified_at`. |
| `verified_at` | RFC3339 string | tool | yes | Stamped by verification tool on success. No other writer may set this. |
| `verified_run_id` | ULID string | tool | yes | ULID of the onboarding run that stamped `verified_at`. Enables provenance tracing. |

Unset fields are `null`, not omitted. Schema is positionally stable so `jq` filters can rely on key presence.

### Confirmed results

Verified companies are appended to the seed file by `apps/tools/cmd/onboard` on the run that stamps `verified_at`.
