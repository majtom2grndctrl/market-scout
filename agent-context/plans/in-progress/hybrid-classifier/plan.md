# hybrid-classifier — plan of record

mode: resumable
status: approved
read at: db1afeb

No source under `apps/tools/` changed between the brief's read-at (2bde04f) and db1afeb. Every symbol the Decisions and Path cite was re-read at db1afeb and exists as described: `selection.Select`/`Criteria.Force`, `boilerplate.NewDBLoader`/`CleanSelected`, `classify.LoadTaxonomy`/`Validate`, `db.Queries.SaveEnrichment`, `validateProvenance`/`buildPayload` in `cmd/mcp/save_enrichment.go`, the `mcp.save_enrichment` advisory-lock wrapper (000026) over `save_enrichment_unlocked` (000043), its Phase C within-payload near-duplicate drop (`c_payload_dup_at` 0.85 on the greater of slug and name `similarity()`), its `empty_dimensions` check, `fetch_runs` (000005), `title_seniority_seeds` (000036), the per-function grant blocks in `action_role.sql`, `apps/web/lib/db/test-dsn.ts`, and `AI_SERVICE_KEY` in `.env.local` (read by nothing).

## Corrections

- The brief places the seniority "ladder" in v9 → v9's ladder has Step 1 (level words and the compound table) and Step 2 (closed scope-signal classes producing `senior` or `director`). The Path seeds candidates from Step 1 and the compound table only, so the tool never produces a Step-2 value. Planning around it by implementing Step 1 exactly: title over body, band lower bound on an explicit separator, and the compound table. No level word judged a level gives `unknown`. This is a clarification of the existing Decision, not a change to it; the two lineages already hold different seniority instruments.
- v9 resolves a compound missing from its table "by judgment, recorded in notes" → the tool has no free-text judgment. Planning around it by returning `unknown` for an adjacent level-word pair missing from the table, and counting such postings in the dry-run output so the probe shows how often it happens.
- "Whichever brief lands second extends the other's repair functions" → no merge or retire repair function exists in source; `profile-and-pins` is an untracked draft. This brief is expected to land first. Planning around it by using plain foreign keys (`ON DELETE RESTRICT`) from the seed and candidate tables, so an unextended repair fails loudly instead of orphaning rows.
- `DATABASE_URL_TEST_ACTIONS` does not exist in `.env.local`, and the action role has not been provisioned on `market_scout_test`. Planning around it with Task 9. The DSN reuses the cluster-level `market_scout_actions` password with the database name swapped, and `action_role.sql` runs against the test database.

## Delegated answers

- Alpha decisions endpoint or `/api/v1/systemone` — `POST /api/alpha/decisions`. It is the endpoint the Transport Decision names, and the decider interface isolates it. Task 2 confirms it; if it rejects the request shape, switching to `systemone` is a transport-only change inside the client.
- Floor, relative ratio, cap, and recording floor per dimension — the Label Decision fixes the relative ratio at 0.5. Starting values for the probe: recording floor 0.05 on every Jev dimension; role floor 0.40; specialization floor 0.50, cap 5; model-judged skill floor 0.50, cap 8; seniority-candidate floor 0.50. The probe sets the final values, and they are pinned in Go under `classify-v1` before any live write.
- Role chunking (not an open question; recorded because the Decision leaves the size unspecified) — roles sorted by slug, split into `ceil(n/50)` balanced chunks. That gives 6 chunks at 291 roles, well under the 255-option cap and inside the ~75-class range the docs demonstrate. The top 5 non-`none_fit` options per chunk go to pass 2.
- Several body candidates judged a level with different values → take the most probable, with ties broken by first occurrence. A title candidate judged a level always wins, as v9's "quote the title" rule says.
- Near-duplicate pre-dedup uses the database's own `similarity()` over the kept labels only (a read-only sqlc query), so the tool's test matches Phase C exactly instead of reimplementing pg_trgm in Go.
- Where candidates are written — a live save sends its run id, candidates, per-label method, and expected-latest precondition in the `save_enrichment` payload, and the function records the `written` outcome and its candidates in the same transaction. Deferred, skipped, and failed outcomes go through a new approved `mcp` function. The MCP handler's payload builder whitelists fields, so agent-sent extras never reach the function.

## Implementation notes

- Title masking has no word edges. Stripped HTML glues words ("Founding Software EngineerYou'll") and longer forms carry the title ("software engineering"); both leaked past an edge-bounded match on real data. The mask replaces title text wherever it appears, so "software engineering" becomes "[—]ing".
- Usage decoding reads `input_tokens`, `prompt_tokens`, and `cost` because the alpha endpoint's usage field names are undocumented. Task 2 confirms which the endpoint sends.
- Dry-run connects on `DATABASE_URL_RO`; it never needs write access.

## Owner gate

Probe acceptance: *not yet recorded.* No commit touching `apps/tools/internal/db/migrations/`, `cmd/mcp/save_enrichment.go`, or any write path lands before this line records it, with date and commit.

## AC-to-proof

Test names are the planned filters; each gets confirmed to match at least one test when it lands.

| AC | Proof | Status |
|---|---|---|
| R1 Role state never contains the title or the level-stripped title, even when the body repeats it | `go test ./cmd/classify -run TestMaskTitle` | achievable as stated |
| R2 Every live role appears in exactly one pass-1 question; pass 2 offers leaders plus `none_fit`; stable chunks; names only; a mid-run mint is not offered (P14) | `go test ./cmd/classify -run TestRoleTournament` | achievable as stated |
| R3 Pass-2 `none_fit` or a top role below the floor defers, writes nothing, and records the reason | `go test ./cmd/classify -run TestRoleDefer`; integration `-run TestClassifyRun_Defers` | achievable as stated |
| R4 A written outcome stores the full pass-2 distribution; a deferred outcome stores its candidates | `go test -tags=integration ./cmd/classify -run TestClassifyRun_StoresCandidates` | achievable as stated |
| A1 A Jev failure after retries fails only that posting; others still write | `go test ./cmd/classify -run TestPostingAtomicity`; integration `-run TestClassifyRun_FailedPosting` | achievable as stated |
| A2 429 or 529 retried after `retry-after`; exhausted retries fail the posting | `go test ./internal/enrich/jev -run TestClient_Retry` | achievable as stated |
| A3 Whitespace-identical requests share one Jev call and labels; one differing character gives two (P6); two runs send their own (P5); a failed shared request fails every sharer (P7) | `go test ./cmd/classify -run TestRequestSharing` | achievable as stated |
| A4 Deferral excluded under the same version, selectable after a bump, still unclassified in `enrichment_preview`; failed re-selected (P1); a changed description stays excluded (P3); forced run selects | `go test -tags=integration ./internal/enrich/selection -run TestSelect_Deferral` | achievable as stated |
| A5 A term retired mid-run fails only its posting; the next run selects it again | `go test -tags=integration ./cmd/classify -run TestClassifyRun_RetiredMidRun` | achievable as stated |
| L1 Floor, half-of-top, and cap boundaries | `go test ./cmd/classify -run TestLabelRule` | achievable as stated |
| L2 Only the more probable of a `save_enrichment` near-duplicate pair is sent; save reports nothing dropped | `go test -tags=integration ./cmd/classify -run TestNearDuplicatePredup` | achievable as stated |
| L3 No surviving specialization or skill still writes, with empty arrays | `go test ./cmd/classify -run TestLabelRule_ZeroSurvivors`; integration write | achievable as stated |
| L4 Recording floor boundary | `go test ./cmd/classify -run TestRecordingFloor` | achievable as stated |
| L5 Go, R, C, C++, C# match only under context rules (must-match and must-not fixtures) | `go test ./internal/enrich/skillseed -run TestLexical_ContextRules` | achievable as stated |
| L6 On an unlike taxonomy, the generator reaches every lexically distinctive skill, picks the `noul` head by that database's link mass, and applies a rule only where its skill exists | `go test -tags=integration ./internal/enrich/skillseed -run TestGenerate` | achievable as stated |
| S1 v9 examples against a fake Jev: title level, body level, "senior stakeholders" not a level, "Chief of Staff" no level, `unknown` with no note | `go test ./internal/enrich/seniority -run TestSeniority_V9Examples` | achievable as stated |
| S2 A judged-not-level phrase gives nothing; a judged-level phrase gives the v9 value and a verbatim note | `go test ./internal/enrich/seniority -run TestSeniority_Judgment` | achievable as stated |
| D1 Re-derive makes zero Jev calls; thresholds re-applied to stored candidates; rule and lexical labels copied; bumped version; names the source run | `go test -tags=integration ./cmd/classify -run TestRederive` | achievable as stated |
| D2 Re-derive skips on a stale latest (P8); defers under a raised floor (P9); writes a source deferral under a lowered floor (P10); a retired term fails the posting | `go test -tags=integration ./cmd/classify -run TestRederive_Ordering` | achievable as stated |
| W1 A written role carries exactly its existing dimensions; no new `canonical_role_dimensions` rows | `go test -tags=integration ./cmd/classify -run TestClassifyRun_EchoesDimensions` | achievable as stated |
| W2 No taxonomy rows added; every `new_taxonomy` empty | `go test -tags=integration ./cmd/classify -run TestClassifyRun_PickOnly` | achievable as stated |
| W3 On the action role, instrument, candidates, run reference, pinned version, and response model persist | `go test -tags=integration ./cmd/classify -run TestClassifyRun_Persists` | achievable as stated |
| W4 A skill-shaped save succeeds with `agent`; `run_id`, `candidates`, `method` added through MCP store nothing | `go test -tags=integration ./cmd/mcp -run TestSaveEnrichment_AgentShape` | achievable as stated |
| W5 The no-role example returns `no_roles` through MCP and SQL and writes nothing; one role succeeds | `go test ./cmd/mcp -run TestSaveEnrichment_NoRoles`; integration `-run TestSaveEnrichmentSQL_NoRoles` | achievable as stated |
| W6 A re-run appends and leaves the earlier row unchanged | `go test -tags=integration ./cmd/classify -run TestClassifyRun_AppendOnly` | achievable as stated |
| W7 Outcomes account for every selected posting; counts match; an empty run completes at zero; an interrupted run stays `in_progress` with its outcomes intact | `go test -tags=integration ./cmd/classify -run TestClassifyRun_Lifecycle` | achievable as stated |
| W8 The run stores thresholds and four hashes; one input changes only its hash | `go test ./cmd/classify -run TestRunHashes` | achievable as stated |
| W9 Dry-run leaves tables unchanged; a later live run selects its deferrals and sends its own requests (P4) | `go test -tags=integration ./cmd/classify -run TestDryRun` | achievable as stated |
| W10 Dry-run JSONL: one line per posting with outcome, labels with instrument and probability, seniority and note, request keys | `go test ./cmd/classify -run TestDryRun_JSONL` | achievable as stated |
| W11 Seed-hash warning on change under one version, none unchanged, none on a new version's first run (P12) | `go test -tags=integration ./cmd/classify -run TestSeedHashWarning` | achievable as stated |
| W12 New integration tests fail before connecting on a non-`_test` DSN, skip when unset | `go test ./internal/testutil -run TestTestDSN` | achievable as stated |
| W13 The migration's down and up round-trip on the test DB; `action_role.sql` re-runs and grants the new functions; zero-role rows unchanged, legacy rows null (P13) | Scripted round-trip on `market_scout_test` plus `go test -tags=integration ./internal/db -run TestMigration0044` | achievable as stated |
| W14 After the pilot, no `cmd/batch-enrich`, and no file outside `agent-context/plans/` names `cmd/batch-enrich`, `codex_exec`, or `bin/batch-enrich` | `rg` grep gate | achievable as stated |
| M1 Owner-adjudicated gold set of about 100 postings as JSONL, v9, titles hidden | owner, gold labelling run and adjudication | manual |
| M2 Probe over about 150 postings reports every listed measure and passes every kill criterion | owner, two paid dry-runs plus `classify probe-report` | manual |
| M3 Description arm on split halves; owner decides on a description column | owner, paid dry-run with `--role-descriptions` | manual |
| M4 No migration, save change, or write path before recorded probe acceptance (git-history gate) | `git log` over the gated paths vs. the Owner gate line | manual |
| M5 Pilot of about 40 postings persists instruments, candidates, and run refs; counts match; cost within estimate; re-derive under a bumped version | owner, live run and re-derive | manual |

## Tasks

Commits: one per completed task, carrying its `plan.md` row update. Every paid command is owner-run.

| # | Task | Owner | Depends on | Status |
|---|---|---|---|---|
| 1 | **First slice.** `internal/enrich/jev` client: plain `net/http`, the decisions request and response types, `retry-after` retries on 429 and 529, typed errors. `cmd/classify` holds the decider interface, title masking, the role tournament, per-run request sharing keyed on whitespace-stripped state and questions, and a dry-run over `selection.Select` plus `boilerplate.CleanSelected` that emits JSONL. Fake decider and `httptest`. AC R1, R2, A2, A3; R3, A1, and W10 in unit form | integrating executor | — | done: `go test -race ./cmd/classify ./internal/enrich/jev ./internal/enrich/seniority` — `TestMaskTitle`, `TestRoleTournament` (incl. P14), `TestRoleDefer`, `TestPostingAtomicity`, `TestRequestSharing` (P5–P7), `TestDryRun_JSONL`, `TestClient_Retry`. Free offline check on the dev DB (read-only role, no Jev calls): selection and boilerplate reads work; 291 roles → 6 chunks; masked states 0.5k–11k chars; 0 title leaks over 300 postings (117 had the title in the body) |
| 2 | **First-slice check.** A paid dry-run over about 10 postings: request shape accepted, context budget, dated model id reported, and role answers read against the full taxonomy. Results recorded here. Stop and report if the endpoint, budget, or accuracy premise fails | owner (command supplied) | 1 | |
| 3 | **Label rules.** Floor, half-of-top, and cap; recording floor; near-duplicate pre-dedup through the database's `similarity()`; zero survivors allowed. AC L1, L3, L4; L2 in unit form | integrating executor | 2 | |
| 4 | **Skill seed and specializations.** `internal/enrich/skillseed`: generator from live skills (lexical aliases, `noul` head by link mass, curated context rules for short names), lexical matcher, seed file I/O for phase 1. Specialization `noul` over every live specialization. AC L5, L6 (L6's database half lands after Task 9) | integrating executor | 3 | |
| 5 | **Seniority.** `internal/enrich/seniority`: tool-owned level-word list from v9 Step 1 and the compound table, candidate extraction from title and body, a Jev `noul` per candidate, Step-1 resolution, and a v9-format note. AC S1, S2 | integrating executor | 2 | |
| 6 | **Dry-run complete and probe tooling.** Full per-posting pipeline under `errgroup.SetLimit`; option-set, seed, and question hashes plus thresholds in the JSONL header; token-based cost accounting; a gold-sample export (title-masked JSONL); a `probe-report` subcommand for every M2 measure; a `--role-descriptions` input for the description arm. AC W8, W10 | integrating executor | 3, 4, 5 | |
| 7 | **Gold set.** About 100 fresh postings labelled under v9 with titles hidden, through subscription-session subagents, to JSONL. Owner adjudicates disagreements. AC M1 | owner (executor may run the labelling on session tokens when authorized) | 6 | |
| 8 | **Probe.** Two paid dry-runs over about 150 postings, the description arm, and `probe-report`. Tune thresholds and pin them under `classify-v1`. Owner accepts or stops against the kill criteria; acceptance recorded in *Owner gate*. AC M2, M3 | owner | 7 | |
| 9 | **Phase 2 opens. Test harness.** Go `_test`-suffix DSN guard in `internal/testutil`; `DATABASE_URL_TEST_ACTIONS`; action role provisioned on `market_scout_test`. AC W12 | integrating executor | 8 (gate) | |
| 10 | **Extract.** `validateProvenance`, `buildPayload`, and result decoding move from `cmd/mcp` to an `internal/` package, in a behavior-preserving commit | integrating executor | 9 | |
| 11 | **Migration 000044 and save extension.** Method columns, candidate, run, run-outcome, and seed tables; `classifications.run_id`; outcome and run functions in `mcp`; `save_enrichment` takes method, candidates, run id, and expected-latest under the lock, and rejects zero roles; `classify.Validate` gains `no_roles`; `action_role.sql` grants; sqlc; applied to both databases. AC W4, W5, W13 | integrating executor | 10 | |
| 12 | **Deferral exclusion.** Opt-in selection criterion keyed on lineage and version; `Force` lifts it; `enrichment_preview` unchanged. AC A4 | integrating executor | 11 | |
| 13 | **Live writes.** Run lifecycle (`in_progress`, outcomes, closing counts), seed table from the database, seed-hash warning, echoed dimensions, mid-run retirement failure, candidates stored with outcomes. AC R3, R4, A1, A5, L2, W1–W3, W6, W7, W9, W11 | integrating executor | 11, 12 | |
| 14 | **Re-derive.** Subcommand over a source run's stored candidates, expected-latest precondition, bumped version. AC D1, D2 | integrating executor | 13 | |
| 15 | **Pilot.** Live run over about 40 postings, then re-derive under a bumped version. AC M5 | owner | 14 | |
| 16 | **Remove the old runner.** Delete `cmd/batch-enrich`; update `Makefile`, `README.md`, `.gitignore`, `.claude/settings.json`, and the preflight, data-audit, and implement-task skills; developer-guide §2 and §6.2 references. AC W14 | integrating executor | 15 | |
| 17 | **Preflight, review, land.** `/preflight`, `/review-panel`, `/fix-findings`; result column; durable `lib/` updates | integrating executor | 16 | |
