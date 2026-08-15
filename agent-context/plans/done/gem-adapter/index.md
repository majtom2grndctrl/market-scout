# Gem Adapter — Build Brief

## Goal

Add a Gem ATS adapter to the fetcher so postings on `jobs.gem.com/<slug>` boards
are ingested alongside Greenhouse, Lever, Ashby, Workday, and Workable. Gem is
appearing on an increasing share of boards encountered while scouting.

Gem returns **full descriptions in the list response**. Unlike Workday and
Workable, it ships complete — no deferred description-fetch follow-up, and its
postings classify immediately.

## Wire shape — probed live 2026-08-12

`GET https://api.gem.com/job_board/v0/{slug}/job_posts/`

No auth, no headers required. Returns a **bare JSON array** at the top level —
not an envelope object (contrast Workable's `{"jobs": [...]}`). Trailing slash
optional; both forms return 200 with a byte-identical body and no redirect.

| Condition | Response |
|---|---|
| Valid slug | 200, array of postings |
| Valid slug, no openings | 200, `[]` (confirmed against `anysphere`) |
| Unknown slug | 404 with a JSON error envelope |
| Wrong-cased slug | 404 — see below |

**Gem slugs are case-sensitive.** `supio` → 200; `Supio`, `SUPIO`, `sUpIo` → all
404. The `jobs.gem.com` front door behaves identically. This is Lever's
signature, not Greenhouse's, and it drives the normalization decision below.

Two boards probed. All 15 top-level keys are present on all 11 postings, but
"present" is not "populated" — `location.name` and `offices[].location.name` are
empty strings on some postings, and `departments[].parent_id`,
`offices[].parent_id`, and `offices[].parent_office_external_id` are JSON `null`
on every posting. No key differs between the two boards.

```
curl -s "https://api.gem.com/job_board/v0/supio/job_posts/"   # 7 postings
curl -s "https://api.gem.com/job_board/v0/gem/job_posts/"     # 4 postings
```

## Boundary inventory

| Wire key | Type | → `domain.Posting` | Notes |
|---|---|---|---|
| `id` | string | `SourceID` | **Required — error if empty**, wrapped `gem:`, naming the index. Opaque: base64-ish on supio (`am9icG9zdDqse7B…`), numeric on gem (`4965519002`). Do not validate its format. |
| `absolute_url` | string | `SourceURL`, `JobURL` | `https://jobs.gem.com/{slug}/{id}` on all 11 postings. Required — error if empty. |
| `title` | string | `Title` | |
| `content_plain` | string | `DescriptionText` | Pre-flattened, entity-decoded, `\n` only. Prefer it; fall back to `htmlToPlainText(content)`. Same shape as `lever.go:244-252` — reuse it. |
| `content` | string | — | HTML. Fallback source only. |
| `first_published_at` | timestamp | `PostedAt`, `SourceFirstPublishedAt` | Use this, not `created_at`. |
| `updated_at` | timestamp | `SourceLastModifiedAt` | |
| `created_at` | timestamp | — | Record-creation time, earlier than publish. Survives in `RawData`. |
| `location` | `{name}` | `LocationText` | Empty on 4 of 7 supio postings — all the 2-office hybrid ones. See the location rule. |
| `offices` | array | `LocationTexts` | Full shape `{id, name, location:{name}, parent_id, parent_office_external_id, child_ids, child_office_external_ids}`. No parent/child nesting observed; those fields are null/empty on every posting and need no decoding. |
| `location_type` | string | `WorkplaceType` | Observed: `hybrid`, `remote`, `in_office`. Map `in_office` → `onsite` for the `workplace_type` CHECK (`000001_initial_schema.up.sql:44`). Unknown value → warn and leave nil. |
| `employment_type` | string | `EmploymentType` | Only observed value across all 11 postings is `full_time` (n=1 distinct). Do not treat the other four schema values as confirmed. |
| `departments` | array | `Department` | First entry's `name`, guarded by `len(departments) > 0`. Exactly one entry on every observed posting; multi-department is unobserved. |
| `internal_job_id`, `requisition_id` | string | — | Not mapped. Survive in `RawData`. |

All timestamps are uniformly `YYYY-MM-DDTHH:MM:SS.mmmZ` — three fractional
digits, literal `Z`, never a numeric offset. Parse with `time.RFC3339Nano`,
matching `greenhouse.go:156` and `ashby.go:177`.

No compensation key exists anywhere in the nested structure on either board. All
four `Compensation*` fields stay nil.

## Decisions

**Board token: pass through unchanged.** Gem joins the `lever` case in
`NormalizeBoardToken` — return the token as-is. Do **not** add `gem` to the
`case "greenhouse", "ashby", "workable"` lowercase branch; that file's doc
comment warns against exactly this, and lowercasing would 404 any Gem board whose
canonical slug carries uppercase. Record the probe evidence (`supio` → 200,
`Supio`/`SUPIO` → 404) in the doc comment alongside Lever's.

For `ValidateBoardToken`, Gem joins the **loose** class (any non-empty token),
alongside Greenhouse/Lever/Ashby. The strict lowercase-slug pattern Workable uses
would reject valid mixed-case Gem tokens.

**Location.** Resolve in this order:

1. Build `LocationTexts` from `offices`, taking `office.location.name` for each
   office that has a non-empty one. Office `name` is a label with modality baked
   in ("Seattle Hybrid"), not a place — drop label-only offices. Modality already
   lives in `workplace_type`, so the label carries no location signal.
2. If that yields nothing and `location.name` is non-empty, set
   `LocationTexts = []string{location.name}`. This preserves the no-NULL-skew
   parity `greenhouse.go:134-140` and `workable.go:149-153` maintain — every
   posting with a location gets a non-nil slice.
3. `LocationText` is `LocationTexts[0]` when the slice is non-empty.
4. No location anywhere leaves both fields nil.

On supio's hybrid postings this yields `LocationText = "San Francisco, United
States"` and `LocationTexts = ["San Francisco, United States"]`. The Seattle
office is dropped because Gem supplies no place-name for it.

`offices: []` on the wire maps to **nil**, not an empty slice — Gem's empty array
carries no signal Greenhouse's absent field doesn't. (`domain.Posting` treats the
distinction as load-bearing; this settles it for Gem.)

**Timestamp parse failure aborts the fetch**, matching `greenhouse.go:155-168`
and `ashby.go:176-186`. Gem's format is uniform across both probed boards, so a
parse failure means schema drift worth failing loudly on rather than a silently
NULL `posted_at`.

**Employment type** routes through an alias map keyed by
`stripNonAlphaNumLower`, seeded from `workableEmploymentAliases`
(`workable.go:192-198`) so casing and punctuation variants collapse. Unknown
value → warn and leave nil. Note in a comment that the four non-`full_time`
entries are speculative — Gem's casing for them is unobserved.

**Pagination: none.** `?per_page=2&page=1`, `?limit=2&offset=0`, and
`?content=true` all return the full board. Add `gemSanityCeiling = 1000` with a
warn-not-fail at `len(jobs) >= ceiling`, mirroring `workableSanityCeiling`
(`workable.go:18-22, 99-102`).

## Non-goals

- The per-posting endpoint (`.../job_posts/{id}/`, returns 200). `content_plain`
  is already in the list response.
- Pagination handling.
- Compensation extraction — nothing on the wire to extract.
- A Gem-specific HTML flattener or string helpers. `httpFetch`,
  `htmlToPlainText`, `ptrIfNonEmpty`, `stripNonAlphaNumLower`, and
  `joinNonEmpty` are already package-level in `ats` — call them, do not
  redefine. (`joinNonEmpty` lives in `workable.go:218`; copying that file
  wholesale is a redeclaration build error.)
- Normalizing the U+00A0 and U+202F codepoints that survive in `content_plain`.
  Noted so the implementer doesn't chase them; leave for a boilerplate-stripping
  decision.

## Files to touch

Follow `workable.go` for structure — single-shot GET, `httpFetch`, per-job
`json.RawMessage` so `RawData` preserves the original payload.

| File | Sites |
|---|---|
| `internal/ats/gem.go` | new — `Gem`, `NewGem`, `newGemWithBaseURL`, `FetchPostings` |
| `internal/ats/gem_test.go`, `testdata/gem/` | new — see fixtures below |
| `internal/ats/greenhouse.go` | package doc comment (line 1) |
| `cmd/fetcher/main.go` | adapters map (90-96) |
| `internal/atsdetect/detect.go` | **four sites**: `SupportedATS()` 51-53, `isSupportedATS()` 246-253, `ValidateBoardToken` switch 74-81, `detectionRules` 200-239 |
| `internal/atsdetect/normalize.go` | `case "lever"` pass-through branch + doc comment |
| `internal/atsdetect/detect_test.go` | `TestSupportedATS_StableOrder` (258) — the one test that hard-fails |
| `internal/atsdetect/normalize_test.go` | add a Gem row asserting `NormalizeBoardToken("gem", "Supio") == "Supio"` |
| `cmd/mcp/main.go` | ATS enum (236) and `board_token` description (237) — note case-sensitivity |
| `cmd/mcp/add_company.go` | probe factory switch (186-202) |
| `cmd/onboard/probe.go` | `adapterFor` switch (57-72), `atsNewGem` factory var (77-83), doc comment (55) |
| `cmd/onboard/seed.go` | `titleCaseATS` display-name switch (214-233) |
| `internal/db/seeds/companies.sql` | `-- Gem (August 2026)` section + the Supio row |
| `agent-context/lib/project.md` | ATS targets section (62-72) |
| `agent-context/lib/watchlist.md` | seven ATS enumerations: lines 20, 21, 73, 83, 96, 109, 198 |
| `README.md` | ATS table (76-80) — mark tokens case-sensitive, as Lever is |

`detect.go`'s two supported-set switches are the trap: `ValidateATS` calls
`isSupportedATS`, **not** `SupportedATS()`. Editing only the latter leaves
`add_company` and every `ValidateBoardToken` call rejecting `ats="gem"` while
`TestSupportedATS_StableOrder` passes.

`cmd/mcp` and `cmd/onboard` tests do not enumerate the full ATS list — only
`detect_test.go:258` does.

## Fixtures

Record `supio` and `gem` verbatim from the live endpoints. Derive the negative
cases by trimming a recorded response — never hand-author from the Go struct.
This is the `testdata/workable/` pattern. Load via `loadAdapterFixture(t, "gem",
name)` (`helpers_test.go:13`).

| File | Source | Exercises |
|---|---|---|
| `jobs_full.json` | supio, verbatim | happy path, hybrid + remote, empty `location.name` |
| `jobs_gem_board.json` | gem, verbatim | numeric `id`, `in_office` — **required**, supio has neither |
| `jobs_empty.json` | `[]` | valid board, no openings |
| `jobs_missing_id.json` | derived | required-field error |
| `jobs_missing_absolute_url.json` | derived | required-field error |
| `jobs_no_locations.json` | derived | all locations empty → both fields nil |
| `jobs_no_content_plain.json` | derived | **the `htmlToPlainText(content)` fallback is unreachable from any real fixture** — `content_plain` is non-empty on all 11 live postings |

## Done when

Observable by `go test ./...`:

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass.
- [ ] The supio hybrid posting decodes to exactly
      `LocationText = "San Francisco, United States"` and
      `LocationTexts = ["San Francisco, United States"]`. Assert the values, not
      that a branch was taken.
- [ ] `first_published_at` populates **both** `PostedAt` and
      `SourceFirstPublishedAt`; `updated_at` populates `SourceLastModifiedAt`;
      `created_at` maps to nothing.
- [ ] The `gem` board fixture yields a posting with the numeric `id` intact as
      `SourceID` and `WorkplaceType = "onsite"` from `in_office`.
- [ ] `content_plain` empty falls back to `htmlToPlainText(content)`.
- [ ] Missing `id` and missing `absolute_url` each produce a wrapped `gem:`
      error naming the index.
- [ ] An invalid board_token (empty, or containing `/`, `?`, `#`, `://`)
      returns a wrapped `gem:` error **before any HTTP request** — the
      `workable_test.go:169-198` pattern. At the fetcher level this counts the
      company as failed, not skipped; no new skip path.
- [ ] All four `Compensation*` fields nil; `RawData` byte-preserved.
- [ ] `NormalizeBoardToken("gem", "Supio")` returns `"Supio"` unchanged.

Manual verification — **not covered by `go test ./...`**, needs live Postgres and
a live Gem call:

- [ ] Add the Supio row to `internal/db/seeds/companies.sql` and apply it. The
      `atsdetect` and `cmd/mcp` changes must land first, or `add_company`
      rejects `ats='gem'`.
- [ ] `go run ./cmd/fetcher` completes without error and writes 7
      `posting_snapshots` rows for Supio, each with non-NULL `title`, `job_url`,
      `posted_at`, `workplace_type`, `description_text`, `location_text`, and
      `location_texts`.

Docs:

- [ ] `project.md` says six adapters are live and the ATS table has a Gem row
      noting case-sensitive tokens and that descriptions ship in v1.
- [ ] `watchlist.md`'s seven ATS spots updated, including the probe-criteria row:
      HTTP 200, bare JSON array, `[]` for a valid empty board, 404 on unknown or
      wrong-cased slug.
- [ ] `README.md` ATS table has a Gem row.

## Notes

Land the `project.md` / `watchlist.md` / `README.md` edits in the implementation
commit. Per `style-guide.md`, the durable layer — external API contracts,
pagination semantics, auth model — belongs in `agent-context/lib/`, not in a plan
doc that moves to `done/` and stops being maintained.

`supio` (7 postings) is the seed row. Gem's own board (`gem`, 4 postings) is
required as a second fixture, not optional — it is the only source of the
numeric-`id` and `in_office` cases.

**Why label-only offices are dropped.** Supio's Seattle office has
`name: "Seattle Hybrid"` and no `location.name`, so the adapter emits no Seattle
entry. The label is not a place: its modality half is already captured, more
reliably, by `location_type` → `workplace_type`, and its place half ("Seattle")
would have to be recovered by parsing customer-authored free text. `raw_data` is
`jsonb`, so the full `offices` array is preserved and re-parseable if that ever
becomes worthwhile. Place extraction is a read-model concern, not an adapter
concern — see `agent-context/plans/in-progress/workplace-type-derivation/`, which
covers modality across all sources and scopes place extraction out with
evidence.
