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
- [ ] A local agent can follow the documented workflow: find careers page or ATS/network URL, call `detect_ats`, call `add_company` with the selected pair, and rely on `add_company` probe failure to prevent bad inserts.
- [ ] From `apps/tools`: `go test ./cmd/onboard ./cmd/mcp ./internal/atsdetect -count=1` passes.
- [ ] From `apps/tools`: `go test ./...` passes.

## Tasks

### Task 1: Extract ATS detection package

Create a small internal package for ATS evidence parsing and token validation. Move the current URL-pattern rules out of `cmd/onboard` instead of copying them. The package owns the supported ATS list, URL-pattern detection, and strict token validation rules currently embedded in `cmd/mcp/add_company`.

The package must not import `cmd/*`, `database/sql`, MCP libraries, or browser tooling. It is pure parsing and validation. It may return typed results and errors that callers map into their own JSON or sidecar statuses.

### Task 2: Rewire existing callers

Update `cmd/onboard` to call the shared package for careers URL detection. Its externally visible behavior should not change: sidecar statuses, Workday token derivation, Workable lowercasing, and ATS probe semantics remain the same.

Update `cmd/mcp/add_company` to call the shared package for supported ATS checks and board-token validation. `add_company` still owns request trimming, careers URL validation, live ATS probing, DB insertion, and its current action envelope.

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
}

func DetectURL(rawURL, sourceKind string) (Detection, error)
func DetectEvidence(careersURL string, observedURLs []string) (Result, error)
func ValidateATS(ats string) error
func ValidateBoardToken(ats, boardToken string) error
```

`DetectEvidence` normalizes and validates URL inputs, runs the same first-match pattern order documented in `watchlist.md`, dedupes identical `(ats, board_token)` matches, and reports ambiguity when distinct supported pairs remain.

`cmd/onboard` maps `Result.Status == "unsupported-ats"` to its existing sidecar status. `cmd/mcp` maps the same result to the `detect_ats` JSON envelope.

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
| Pattern | `Pattern` | `"pattern"` | Stable rule label, e.g. `greenhouse_boards` |
| Errors | `Errors` | `"errors"` | Action-style validation errors with `path`, `code`, `message` |

## Open Questions

1. Should `detect_ats` accept only absolute HTTP(S) URLs, or should it tolerate bare hosts copied from network tooling? Lean: require absolute URLs for v1; agents can add `https://` when needed.
2. Should `detect_ats` select the first match when evidence conflicts, or require caller choice? Lean: require caller choice. Conflicts are rare and should stay visible.
3. Should repeated real-world use promote this workflow into a dedicated Codex skill? Lean: defer. `watchlist.md` is enough until the procedure proves too long for context docs.
