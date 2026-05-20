# Company Onboarding Pipeline

> **Read this when:** building the company-onboarding verification tool, or running the first pass over `research/geekwire-200.md`.
> **Key invariant:** annotation is a structured sidecar; the research file is a frozen artifact. Only the verification tool may stamp a record as verified.
> **Related:** `agent-context/lib/watchlist.md` (sourcing procedure, ATS detection, dedup, board-token probes), `agent-context/lib/project.md` §ATS targets, `internal/db/seeds/companies.sql`, `internal/ats/`.

---

## Goal

Replace inline markdown annotation of research lists with a structured JSONL sidecar plus a verification tool (`cmd/onboard`) that probes each record, stamps verified ones, and emits seed rows. Ship the tool, then run it over `research/geekwire-200.md` as the first input.

Two deliverables in one spec:

1. `cmd/onboard` — Go binary that reads a sidecar JSONL, probes URLs and ATS endpoints, writes back to the same JSONL, and produces seed rows for verified companies on supported ATS platforms.
2. `research/geekwire-200.jsonl` — sidecar for the GeekWire 200, populated by an annotating agent and then run through `cmd/onboard` until every record is either verified or carries a terminal status.

## Scope

### In scope

- Sidecar JSONL format and schema (see *Sidecar schema* below).
- `cmd/onboard` binary: input JSONL → in-place annotation + seed-row emission.
- ATS detection heuristics — defined in `watchlist.md` §ATS detection.
- Status taxonomy — defined in `watchlist.md` §Research file annotation; superset of the prior taxonomy with `invalid-token` and `stale-needs-merge` added.
- One annotation pass over `research/geekwire-200.md` via the new pipeline:
  - Agent generates `research/geekwire-200.jsonl` from the frozen source.
  - `cmd/onboard` runs to verification.
- Seed rows for verified companies appended to `internal/db/seeds/companies.sql`.

### Out of scope (non-goals)

- Editing `research/geekwire-200.md`. Frozen artifact.
- Merge logic for companies already in the DB whose data has drifted (domain changed, ATS migrated, fetcher stale). Surfaces as `stale-needs-merge` for human review.
- Industry normalization. Source `industry` is preserved verbatim in the sidecar; the canonical industry written to the seed file is the seeder's call.
- Re-running onboarding after the first GeekWire 200 pass.
- Adapter coverage beyond `watchlist.md` §What to capture.
- Per-posting description fetch for Workday/Workable (already deferred in `project.md`).

## Sidecar schema

Full field schema: `watchlist.md` §Research file annotation (Sidecar record schema). Records are keyed by `rank`. Three layers, distinguished by who may write each field:

| Layer | Writer | Lifecycle |
|---|---|---|
| Source | Annotating agent on initial sidecar generation | Immutable thereafter — copied verbatim from the research file |
| Annotation | Annotating agent (any pass) | Mutable until the record reaches a terminal state |
| Tool-stamped | `cmd/onboard` only | Set on successful verification or terminal failure |

Preconditions for `cmd/onboard`: `url` must be set for every in-progress record; `careers_url` must be set unless `status = no-careers`.

### Invariants

- A record is verified iff `verified_at != null` and `status == null`.
- A record is terminal iff `status != null` (regardless of `verified_at`, which should be null in this case).
- A record is in progress iff `verified_at == null` and `status == null`.
- `cmd/onboard` exits non-zero if any in-progress record has not reached a terminal state by the end of the run (excluding records that lack the inputs needed to probe — see tool contract).

## Status taxonomy

Canonical taxonomy: `watchlist.md` §Research file annotation. Statuses are terminal. Once set, a record is not reprobed.

## Verification contract

Binary: `cmd/onboard`. Follows the repo's `cmd/<verb>` convention (cf. `cmd/fetcher`, `cmd/batch-enrich`, `cmd/strip-boilerplate`).

### Inputs

- A sidecar JSONL path (positional argument or `-input` flag — implementor's call).
- DB connection via the same configuration mechanism the rest of the Go binaries use.
- Read access to `internal/db/seeds/companies.sql` location (or its directory) for seed-row emission.

### Per-record behavior

For each record, in file order:

1. **Skip if terminal.** `status != null` → no-op.
2. **Skip if already verified.** `verified_at != null` → no-op.
3. **DB dedup check.** Run the dedup sqlc query in `internal/db/queries/` against the `companies` table per `watchlist.md` §Dedup (`(ats, board_token)` match with a `posting_snapshots` row within the last 30 days). Decision matrix:
   - Match on `(ats, board_token)` with recent postings → `status = duplicate`.
   - Match on `(ats, board_token)` with no recent postings, or name match with non-matching ATS → `status = stale-needs-merge`.
   - No match → continue.
4. **Probe careers URL.** GET `careers_url`. Non-2xx and no `ats` set → `status = no-careers`. If `ats` is already set and the careers URL probe fails, log the failure as advisory and proceed to step 5 — the ATS probe is the truth signal.
5. **ATS probe.** Use the URL pattern and success signal from `watchlist.md` §Board token verification. Reuse adapter probe logic from `internal/ats/<adapter>.go` where it exists.
   - Probe succeeds → continue to step 6.
   - Probe fails (HTTP error, malformed response, gated tenant) → `status = invalid-token`.
   - `ats` is null but careers URL matches a known pattern → tool fills `ats` and `board_token` per `watchlist.md` §ATS detection, then re-enters step 5. If `status = unsupported-ats` is already set, the tool does not override it but logs a warning flagging the conflict for human review.
   - `ats` is null and careers URL matches no pattern → `status = unsupported-ats`.
6. **Stamp verified.** Set `verified_at` to the current RFC3339 UTC timestamp; set `verified_run_id`.
7. **Emit seed row.** Append to `internal/db/seeds/companies.sql` (see *Seed-file writeback*).

The homepage URL is not probed. Presence of `url` in the sidecar is taken as the annotator's confirmation that the company is live; absence is a precondition failure (see *Exit semantics*). The ATS probe is the canonical falsifiability gate.

### Seed-file writeback

The tool appends to `internal/db/seeds/companies.sql`. Rationale: the seed file is the canonical source of truth (`watchlist.md` key invariant), and a separate emission file would create a sync gap.

The seed file contains a single multi-row `INSERT ... VALUES (...), (...) ON CONFLICT (ats, board_token) DO NOTHING;`. The tool does not splice into this statement. Instead, it appends a new, self-contained `INSERT ... ON CONFLICT (ats, board_token) DO NOTHING;` statement after the existing one, grouped under an ATS-section comment (e.g. `-- Greenhouse (GeekWire 200, May 2026)`). This avoids trailing-comma errors, keeps `git diff` readable, and matches the style already present in the file.

Idempotence: the `ON CONFLICT (ats, board_token) DO NOTHING` constraint is the DB's dedup gate. The tool additionally checks the `(ats, board_token)` pair in the seed file before appending to avoid emitting a duplicate statement on re-runs. `name` is not part of the dedup tuple; it matches the DB's actual unique constraint. Rationale: two research lists may give different display names to the same board token; using `(ats, board_token)` catches that case while remaining consistent with DB enforcement.

### JSONL writeback

The tool rewrites the sidecar in place. Atomicity is load-bearing: a crash mid-write must not produce a half-written record. Write to a temp file in the same directory; rename on success.

**Exclusive access.** The tool assumes exclusive access to the sidecar during a run. Annotators must not edit the sidecar while `cmd/onboard` is running. The tool does not implement file locking in v1; this is a workflow constraint, not an enforced invariant.

### Exit semantics

| Condition | Exit code |
|---|---|
| All records reached a final state (verified or terminal status). | 0 |
| One or more in-progress records lack inputs the tool requires (e.g. `url` missing). | 2 — precondition failure; structured log lists the ranks needing annotator attention. |
| Probe transport errors not attributable to a specific company (DNS, TCP, TLS, local network down). | 1 |
| Unhandled error. | 1 |

The exit code is the acceptance signal. Model self-attestation ("I checked them all") is not.

### Structured output

Stdout: a JSON summary on completion — counts of verified, each status value, and in-progress remaining. The shape is the implementor's choice but must be machine-readable; the acceptance criteria below assume jq-queryable output.

## Acceptance criteria

### Tool

- [x] `go build ./...` passes with `cmd/onboard` added.
- [x] `go test ./cmd/onboard/...` passes.
- [x] `go vet ./...` and `staticcheck ./...` report no new issues.
- [x] Running `cmd/onboard` against a fixture sidecar with one record per status path (verified, duplicate, stale-needs-merge, invalid-token, unsupported-ats, no-careers, dead) produces the expected `status` or `verified_at` on each.
- [x] Running the tool twice over the same sidecar is a no-op on the second run (no duplicate seed rows, no changed fields on already-final records).
- [x] Writeback uses temp-file + atomic rename; verified by code inspection of the writeback path. Property: any sidecar on disk is either the pre-write or post-write state — no partial JSONL lines.

### GeekWire 200 pass

- [ ] `research/geekwire-200.jsonl` exists, one record per ranked entry in `research/geekwire-200.md`. Duplicate-numbered entries (e.g. Outreach, Boundless, Amperity appear twice in the source) each get a record; later occurrences receive `status = duplicate` from the tool's DB check or from the annotator.
- [ ] After a successful annotator pass + `cmd/onboard` run, the sidecar contains zero in-progress records (`verified_at == null && status == null`).
- [ ] Every record with `verified_at != null` has a corresponding row in `internal/db/seeds/companies.sql`.
- [ ] The tool exits 0.

## Workflow per run

1. Annotator reads the source (or the existing sidecar) and fills in `url`, `careers_url`, `ats`, `board_token`, and any annotator-set `status` values.
2. Run `cmd/onboard <sidecar>`.
3. Inspect the exit code and structured summary.
4. For exit code 2: annotator addresses listed ranks, re-runs.
5. For `stale-needs-merge`: out of scope — human review.

## Open questions

1. **Concurrency.** The tool processes records sequentially in the contract above. The first pass over 200 entries is bounded enough that sequential is fine. If later research lists are larger, a bounded worker pool may be worth adding — defer until pressure surfaces.
2. **Failure replay.** When a probe fails transiently (network blip during the homepage GET), should the tool skip and retry next run? Current contract treats the ATS probe as terminal on failure (`invalid-token`). Open: should we add a non-terminal `transient` state? Lean: no, keep the surface minimal; the annotator can clear `status` to reprobe.
3. **CLI flag matrix.** Beyond `-input`, the tool may want `-dry-run` (skip seed-file writes), `-only-rank N` (probe a single record), `-no-db` (skip the dedup check for offline runs). Add when the operational need is concrete.
