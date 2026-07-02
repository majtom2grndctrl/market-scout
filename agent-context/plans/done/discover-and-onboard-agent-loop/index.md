# Discover And Onboard Agent Loop

## Goal

Give a local agent a reliable path from "interesting company" to "company is in the fetcher set."

Discovery remains an agent/browser workflow. Deterministic URL evidence parsing moves into a small shared Go boundary, exposed through a thin MCP preflight. `add_company` stays the verification and write gate.

## Scope

### In scope

- Extract supported ATS URL detection from `cmd/onboard` into a shared internal package.
- Reuse the shared package from `cmd/onboard` without changing sidecar behavior.
- Reuse shared board-token validation from `cmd/mcp/add_company`.
- Add a non-mutating `detect_ats` MCP tool that parses explicit URL evidence supplied by an agent.
- Document the live discover-and-onboard workflow in `watchlist.md`.
- Unit tests for URL detection, board-token validation, MCP response mapping, and unchanged onboard behavior.

### Out of scope

- Browser automation inside Go or MCP.
- Web search, crawling, or finding careers pages inside Go or MCP.
- Network-log capture inside Go or MCP. The agent supplies observed URLs.
- DB reads or writes from `detect_ats`.
- Changing the `add_company` DB action boundary.
- Seed-file mutation from MCP.
- New ATS adapter support.
- Dedup or stale-merge handling for the live MCP path.
- A new Codex skill. Use `watchlist.md` first; split to a skill only after real workflow friction appears.

## Acceptance Criteria

- [ ] `cmd/onboard` detects the same supported URL patterns as before, including Workday locale stripping and Workable lowercasing.
- [ ] `cmd/mcp/add_company` still rejects the same invalid `ats`, `board_token`, and `careers_page_url` inputs as before.
- [ ] `detect_ats` accepts `careers_url` plus optional ordered `observed_urls`, and returns structured JSON without opening a browser, making HTTP requests, or touching the DB.
- [ ] `detect_ats` returns one selected `(ats, board_token)` only when the supplied URL evidence resolves to a single supported target.
- [ ] `detect_ats` returns an `ambiguous` result when supplied URLs resolve to more than one distinct supported `(ats, board_token)` pair.
- [ ] `detect_ats` returns an `unsupported-ats` result when no supplied URL matches a supported ATS pattern.
- [ ] `detect_ats` returns an `invalid-input` result with an `errors` array and no `selected` when supplied evidence is not an absolute http(s) URL. Bad evidence never yields an MCP transport error.
- [ ] `detect_ats` returns a `detected` match when a supplied URL matches a supported pattern but the captured `board_token` fails syntactic validation — e.g. `apply.workable.com/Acme_Co`, whose token `acme_co` fails the Workable slug rule yet still returns `detected`. Token rejection is left to `add_company`.
- [ ] `atsdetect` imports none of `cmd/*`, `database/sql`, the MCP libraries, or browser tooling (verify via `go list -deps`).
- [ ] The moved logic no longer exists under `cmd/*`: `detectATS` / `atsDetectionRules` are gone from `cmd/onboard`, the board-token and absolute-URL validators are gone from `cmd/mcp/add_company`, and both callers reference `atsdetect`.
- [ ] `detect_ats`'s JSON response matches the Boundary Inventory keys: top-level `status` / `selected` / `matches` / `errors`, each match carrying `ats` / `board_token` / `source_url` / `source_kind` / `pattern`.
- [ ] The `watchlist.md` "Live discover-and-onboard" section documents the loop, and `detect_ats`'s `selected.ats` / `selected.board_token` map directly onto `add_company`'s `ats` / `board_token` inputs. (Manual acceptance: an agent completes find-URL → `detect_ats` → `add_company`, relying on `add_company` probe failure to prevent bad inserts.)
- [ ] From `apps/tools`: `go test ./cmd/onboard ./cmd/mcp ./internal/atsdetect -count=1` passes.
- [ ] From `apps/tools`: `go test ./...` passes.

## Tasks

### Task 1: Extract ATS detection package

Create a small internal package for ATS evidence parsing and token validation. Move the current URL-pattern rules out of `cmd/onboard` instead of copying them. The package owns the supported ATS list, URL-pattern detection, the strict token validation rules currently embedded in `cmd/mcp/add_company`, and the absolute-HTTP(S) URL validator that `detect_ats` and `add_company` share.

The package must not import `cmd/*`, `database/sql`, MCP libraries, or browser tooling. It is pure parsing and validation. It may return typed results and errors that callers map into their own JSON or sidecar statuses.

### Task 2: Rewire existing callers

Update `cmd/onboard` to call the shared package for careers URL detection. Workday token derivation and Workable lowercasing move into `atsdetect`, run by `DetectURL`; onboard stops doing those transforms itself. Its externally visible output stays identical — it now delegates the transform rather than keeping a duplicate. onboard invokes `DetectURL` at both of its existing detection sites, and each keeps its current not-recognized mapping: `no-careers` when the careers probe already failed, `unsupported-ats` when it succeeded but no pattern matched. Sidecar statuses and ATS probe semantics stay onboard's own, unchanged.

Update `cmd/mcp/add_company` to call the shared package for supported ATS checks, board-token validation, and absolute-URL validation. `add_company` still owns request trimming, live ATS probing, DB insertion, and its current action envelope; it now applies those three validation rules through `atsdetect` rather than its own copies.

### Task 3: Add `detect_ats` MCP preflight

Add a non-mutating MCP tool named `detect_ats`. It takes URL evidence only:

| Name | Go field | JSON key |
|---|---|---|
| Careers URL | `CareersURL` | `"careers_url"` |
| Observed URLs | `ObservedURLs` | `"observed_urls"` |

The tool passes the careers URL first, then observed URLs in caller-supplied order, to the shared package. It returns all supported matches and a single selected result only when there is no conflict. Malformed input returns an action-style JSON envelope, not an MCP transport error, unless the server cannot decode the tool call. It does not need either MCP database pool.

`detect_ats` does not probe the ATS endpoint. `add_company` already probes before insert, so a separate probe would double the network call and blur which tool owns verification.

### Task 4: Document the live agent workflow

Update `watchlist.md` with a short "Live discover-and-onboard" section. It should say:

- Agent or human finds the company homepage and careers page.
- Agent may inspect browser redirects, page links, scripts, and network request URLs.
- Agent passes the careers URL and relevant observed URLs to `detect_ats`.
- Agent calls `add_company` with the selected `ats` and `board_token`.
- `add_company` probe success is the falsifiability gate; failed probes insert nothing.
- Seed-file drift remains visible through the existing `add_company` follow-up.

Keep the existing sidecar workflow intact. The live MCP path is for one-off additions; `cmd/onboard` remains the batch sidecar verifier and seed writer.

### Task 5: Tests

Move or recreate tests so shared detection behavior is covered in the shared package. Keep `cmd/onboard` tests focused on process behavior, including dedup after auto-detection. Add MCP tests for `detect_ats` response mapping, ambiguous evidence, unsupported evidence, malformed URLs, and no DB/probe dependency.

## Sequencing

**Phase 1 (sequential):** Task 1 — establishes the shared parsing and validation boundary.
**Phase 2 (sequential):** Task 2 — consumes the shared boundary and proves existing behavior holds.
**Phase 3 (concurrent):** Task 3, Task 4 — MCP preflight and workflow docs can move independently after callers are rewired.
**Phase 4 (sequential):** Task 5 — closes behavior coverage across the new shared package and both callers.

## Rough Sketch

New package: `apps/tools/internal/atsdetect`.

Candidate exported surface:

```go
// Proposed design
type Detection struct {
	Recognized bool
	ATS        string
	BoardToken string
	SourceURL  string
	SourceKind string
	Pattern    string
}

type Result struct {
	Status   string
	Selected *Detection
	Matches  []Detection
	Errors   []ActionError
}

type ActionError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func DetectURL(rawURL, sourceKind string) Detection
func DetectEvidence(careersURL string, observedURLs []string) (Result, error)
func ValidateATS(ats string) error
func ValidateBoardToken(ats, boardToken string) error
func ValidateURL(rawURL string) error
```

`ActionError` mirrors the existing `actionError` in `cmd/mcp/add_company`, so both tools return the same error shape.

`DetectEvidence` validates each URL with `ValidateURL`, then applies the ordered detection rules per URL via `DetectURL` (first pattern match wins, as documented in `watchlist.md`). It dedupes identical `(ats, board_token)` matches across URLs and reports `ambiguous` only when the deduped set holds more than one distinct supported pair. It does not run `ValidateBoardToken` on captured tokens; a match with a malformed token still surfaces as `detected` (see Decisions). `DetectURL` is pure pattern detection — it never errors, returning `Detection{Recognized: false}` for a URL that matches no pattern.

`Result.Status` takes one of four values: `detected`, `ambiguous`, `unsupported-ats`, `invalid-input` — the same set the `detect_ats` JSON envelope uses. `cmd/onboard` works one careers URL at a time, so it calls `DetectURL` and maps `Detection.Recognized == false` to its existing inline `"unsupported-ats"` sidecar literal. `cmd/mcp` calls `DetectEvidence` and maps the returned `Result` to the `detect_ats` JSON envelope.

`DetectEvidence` returns `Result{Status: "invalid-input", Errors: [...]}` with a nil Go error when URL evidence is malformed or not absolute http(s). The Go `error` return is reserved for internal failures. Task 3's transport error fires only when the MCP server cannot decode the tool call.

## Boundary Inventory

| Name | Go field | JSON key | Meaning |
|---|---|---|---|
| Careers URL | `CareersURL` | `"careers_url"` | Primary URL evidence, usually the visible careers page or final ATS URL |
| Observed URLs | `ObservedURLs` | `"observed_urls"` | Ordered URLs observed by the agent in browser redirects, page links, scripts, or network requests |
| Status | `Status` | `"status"` | `detected`, `ambiguous`, `unsupported-ats`, or `invalid-input` |
| Selected match | `Selected` | `"selected"` | Single supported `(ats, board_token)` when unambiguous |
| Matches | `Matches` | `"matches"` | All supported URL matches found in supplied evidence |
| ATS | `ATS` | `"ats"` | `greenhouse`, `lever`, `ashby`, `workday`, or `workable` |
| Board token | `BoardToken` | `"board_token"` | Token passed to `add_company` |
| Source URL | `SourceURL` | `"source_url"` | URL that produced this match |
| Source kind | `SourceKind` | `"source_kind"` | `careers_url` or `observed_url` |
| Pattern | `Pattern` | `"pattern"` | Stable rule label — see list below |
| Errors | `Errors` | `"errors"` | Action-style validation errors with `path`, `code`, `message` |

`detect_ats` uses `careers_url` (generic URL evidence); `add_company` uses `careers_page_url` (its DB-facing field). The divergence is deliberate; the agent maps between them at hand-off.

Pattern labels, one per detection rule: `greenhouse_boards` (boards.greenhouse.io), `greenhouse_job_boards` (job-boards.greenhouse.io), `lever` (jobs.lever.co), `ashby` (jobs.ashbyhq.com), `workday` (*.myworkdayjobs.com), `workable` (apply.workable.com).

## Decisions

**URL input format: absolute HTTP(S) only.** `detect_ats` accepts absolute http(s) URLs and rejects bare hosts. `cmd/mcp/add_company` already enforces this through its absolute-HTTP-URL validator. Task 1 extracts that validation into `atsdetect`, so both callers share one rule. A looser rule in `detect_ats` would reintroduce the drift the extraction removes. Browser-observed evidence — redirects, page links, network requests — is already absolute, so the constraint costs the agent nothing.

**Conflicting evidence requires caller choice; never auto-select.** When deduped evidence resolves to more than one distinct `(ats, board_token)` pair, `detect_ats` returns `ambiguous` with all matches and no `selected`. The calling agent picks. This follows the project's visibility-over-silent-resolution invariant — flag uncertain matches, never silently pick one (see `watchlist.md` §Dedup and the onboarding pipeline's conflict logging).

Two distinct senses of "first match" coexist and must not be conflated:

- **Pattern precedence within a single URL** — the ordered detection rules, first match wins. Deterministic and load-bearing. One URL yields at most one match.
- **Cross-evidence conflict** — the deduped set of distinct `(ats, board_token)` pairs across all supplied URLs has more than one element. This yields `ambiguous`.

The first stays; only the second produces `ambiguous`.

**Token validation belongs to `add_company`, not `detect_ats`.** A URL can match a supported pattern yet capture a token that fails `ValidateBoardToken`. `detect_ats` still reports it as a `detected` match; it does not re-gate token syntax. `add_company` re-validates and probes before insert, so a bad token is caught at the real gate. This keeps `add_company` the single verification gate and stops a second, weaker check from drifting out of sync. `ValidateBoardToken` lives in `atsdetect` for `add_company` to call, not for `detect_ats`.

**Codex skill: defer, with a named flip-trigger.** No dedicated Codex skill now. The workflow lives in `watchlist.md`, which agents read by default, and the loop is short. This follows the project's defer-until-pressure-surfaces invariant, and matches the Scope note listing "A new Codex skill" out of scope. Promote to a skill when the procedure outgrows `watchlist.md`, or when it runs often enough that a slash-command shortcut saves real friction.
