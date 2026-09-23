# hybrid-classifier

Brief · resumable · reads: `agent-context/lib/project.md` §Settled architecture, §Evidence trust tiers · `agent-context/lib/developer-guide.md` §2 Cost map, §2 Test database, §6.2 · `research/enrichment-tool-design-inputs.md` · read at 195f8c9

## Problem

Owner-requested capability. The Go enricher, `cmd/batch-enrich`, shells out to LLM CLIs and writes taxonomy and classification rows directly under owner credentials, a second write route that skips every gate `mcp.save_enrichment` enforces; it stays unused until rewritten. The owner wants an unattended classifier that is cheap per posting, uses the right instrument for each dimension, and keeps the evidence behind each label. When done, an operator runs one Go binary over a selected cohort. It writes one append-only classification per posting through `mcp.save_enrichment`, or writes nothing and records why. Every label records its instrument, and every model judgment keeps its probabilities, so thresholds can be re-tuned without paying for a new model call. The read model shows which classifier wrote each row, and the recent window is reclassified so analyses over it read one classifier. Seniority has one implementation, shared with the LLM skills.

## Decisions

- **New binary `cmd/classify` replaces `cmd/batch-enrich`, removed in the same change.** None of the LLM contract survives: prompt, response schema, hint retries, direct writeback. `codex-native-batch-enrichment` excluded rewriting the runner from its own scope; that plan is implemented and sets no standing rule.
- **OpenRouter serves an automation path, not only chat.** Diverges from `project.md` "Model access: chat surface only". The cost is about $1 per 1,000 postings (`research.md` §Cost). The Codex skill stays the interactive path and the only minting path.
- **One instrument per dimension; quotable evidence gets code, contextual judgment gets the model.** This is the deterministic-vs-agent-judgment axis, and each label records which side produced it.

  | Dimension | Instrument | Options offered |
  |---|---|---|
  | Seniority | Shared Go core porting v9's ladder: level words, then closed phrase lists, else `unknown` | Closed enum |
  | Skills | Lexical match from a skill seed table, plus Jev `noul` for seed-listed non-lexical skills | Only skills the seed table lists |
  | Specializations | Jev `noul` per specialization | Every live specialization |
  | Role | Jev two-pass `choice` | Every live role |

- **Pick-only.** The tool never mints. Option sets are read from the live taxonomy at run start. A skill absent from the seed table is unreachable until seeded; newly minted skills are too. Non-lexical skills outside the `noul` head are unreachable by design, so skill comparisons across classifiers use the vocabulary both can reach.
- **Role reads the description with the title masked.** Masking covers the full title and the title stripped of level words. Title-vs-role divergence is the research measure (`project.md` §Settled architecture, title-stated vs classifier-inferred); an unmasked question measures the title (`research.md` §Title leakage). The seniority core does read the title, as v9's Step 1 does.
- **Role is a tournament over the full taxonomy, at every size.** Pass 1 splits all roles into stable chunks under Jev's option cap, each with `none_fit`, in one request. Pass 2 is one `choice` over each chunk's top five plus `none_fit`. No pruning: a usage floor would silently set the taxonomy. Options are offered by name only.
- **One assigned role; a blend is measured, not labelled.** Diverges from v9's "blended roles are first-class". Skills have no mapping to roles, so a skill mix cannot say which roles a posting blends. The stored pass-2 distribution can: the gap between the top two probabilities measures the blend directly.
- **All or nothing per posting.** If pass 2 returns `none_fit`, the top role is below the floor, or any Jev call for the posting fails after retries, nothing is written. A zero-role row would blank the posting's older role in `latest_classifications`.
- **`mcp.save_enrichment` rejects a classification with zero roles, and so does `classify.Validate`.** This guards every writer, including the MCP skills. Existing zero-role rows stay; storage is append-only.
- **The tool does not re-select its own deferrals.** Each run records a per-posting outcome: written, deferred with reason, or failed. Selection gains an opt-in criterion that excludes postings this lineage deferred under the current `prompt_version`. The skills' selection is unchanged, so they pick deferred postings up as unclassified, and the minting path receives exactly the postings the taxonomy does not cover.
- **Multi-label rule for `noul` dimensions: floor, relative, cap.** A label is kept only when its probability clears the floor, reaches at least half the top label's, and ranks within the cap. Values are tuned for precision on the gold set, not for historical label density. Zero surviving labels is a valid result.
- **Probabilities are stored; labels are applied at write time.** Every Jev candidate above a low recording floor is stored with its probability, including the full pass-2 role distribution. Join tables keep meaning "assigned label", so no existing view changes meaning. A probability is stored once, in the candidate table, never copied onto the join row.
- **Re-tuning is an offline re-derivation.** A re-derive mode reads a prior run's stored candidates, applies new thresholds, recomputes the deterministic instruments, and writes new classifications under a bumped `prompt_version` with no Jev call. The run record names its source run. Candidates below the recording floor are gone, so a floor raised above it cannot be re-derived.
- **One seniority core for both paths.** A Go package implements the v9 ladder once, on boilerplate-cleaned text. `cmd/classify` imports it. The LLM skills get it through a new read-only MCP tool and stop reading seniority themselves, which bumps both skill pins to the next `batch-enrich-v<n>`. The core stays deterministic, so both paths return identical answers for the same posting.
- **Seniority keeps v9's evidence format.** Notes carry `seniority[<tag>]: "<verbatim phrase>"`; `unknown` carries none. The core returns the note line, so no caller formats it.
- **A role is written with its existing dimension set.** `save_enrichment` requires dimensions; echoing the current set adds no `canonical_role_dimensions` rows, so the ratchet (`research/enrichment-tool-design-inputs.md` §Taxonomy interaction) does not grow.
- **Writes go only through `mcp.save_enrichment` on the action role.** The Go-side checks the MCP handler runs (provenance, payload build, result decode) move to an `internal/` package both binaries import. The MCP save tool's surface is unchanged: agents never send instruments, candidates, or run ids.
- **Schema, additive.**
  - An instrument column on the role, specialization, and skill join tables. Legacy rows stay null.
  - A candidate table of term, instrument, and probability per classification.
  - A classification run table, a per-posting run outcome table, and a nullable run reference on `classifications`.
  - A skill seed table listing each reachable skill, its instrument, and its lexical aliases.

  Run rows are written through new approved `mcp` functions, because the action role holds only per-function EXECUTE (`action_role.sql`). The payload extension keeps the external `mcp.save_enrichment(jsonb, text, text)` signature. Undo cost: the down migration drops the tables and columns, discarding stored probabilities and run history.
- **The read model shows the classifier.** `latest_classifications` gains `prompt_version`, `model`, and the run reference, appended so existing consumers keep their columns. This is the read-model placement: lineage is derived once in the view, not rejoined per consumer (`project.md` §Settled architecture).
- **The recent window is reclassified once.** After the pilot passes, the owner runs a backfill that force-reclassifies every posting first seen within three months of the run. Analyses over that window then read one classifier. Selection gains an opt-in first-seen window criterion for it. Older postings keep their LLM rows, visible by lineage.
- **Provenance.** `prompt_version` is a new lineage, `classify-v<n>`, pinned once in Go and bumped by hand for any change to rules, thresholds, or question wording (`developer-guide.md` §6.2: never encode the model). `model` is the id the Jev response reports. Each run records its thresholds and the hashes of the role and specialization option sets, the seed table, and the question wording. At startup the tool warns when the seed hash differs from the last run under the same `prompt_version`.
- **Transport: `net/http` against OpenRouter's decisions endpoint, behind an interface declared in the consuming package.** No SDK, since the endpoint is alpha. The interface lets tests use a fake. The key comes from `AI_SERVICE_KEY`, which nothing else reads today.
- **Dry-run is the probe.** A dry-run mode runs the real pipeline, writes no classification and no run row, and emits per-posting JSONL.
- **Gold set before the probe.** About 100 fresh postings are labelled under the v9 contract with the title hidden, through the subscription skill path. They are written to JSONL, not the database, because title-hidden v9 is not the v9 contract. The owner settles disagreements.
- **Kill criteria stop the build before any live write.** Stop and report to the owner if role top-1 falls more than ten points below the gold set's own agreement, if the high-confidence bucket is not clearly more accurate, or if more than 5% of role answers flip across two runs.
- **Integration tests run against `market_scout_test`.** Guarded on the `_test` suffix, like `apps/web/lib/db/test-dsn.ts`, with the action role provisioned there.
- **Non-goals.**
  - Embeddings; they belong to taxonomy hygiene and title-fit, later, via Ollama.
  - A taxonomy description column; the probe's description arm decides whether one earns a follow-up.
  - Synonym-cluster dedup.
  - The dimension-ratchet fix.
  - Persisting required-vs-preferred skills.
  - A workflow to seed newly minted skills.
  - Measure-engine changes. Lineage in the view plus the backfill keeps the recent window single-classifier; splitting measures by lineage is the engine's own follow-up.
  - Moving the existing Go integration suites to the test database.
  - An LLM fallback, which would be a new brief if the kill criteria fire.

### Agent-facing surface

`infer_seniority` binds the read-only pool and mirrors `strip_boilerplate`: one company, one to 100 of its postings, rules applied to the same boilerplate-cleaned text `cmd/classify` uses.

```json
// call
{"company_id": 42, "selected_ids": [51230, 51231]}
// response
{"ok": true, "company_id": 42, "postings": [
  {"posting_id": 51230, "seniority": "staff", "tag": "step1-title",
   "phrase": "Staff Product Designer",
   "note": "seniority[step1-title]: \"Staff Product Designer\""},
  {"posting_id": 51231, "seniority": "unknown", "tag": null, "phrase": null, "note": null}],
 "errors": []}
// failure: {"ok": false, "company_id": 42, "postings": [], "errors": [{"path": "selected_ids", "code": "invalid_selected_ids", "message": "..."}]}
```

Duplicate, unknown, or cross-company ids fail the whole call, as in `strip_boilerplate`.

## Acceptance

### Automated

**Role question**
- [ ] No role question's state contains the posting's title or the title with level words removed, including when the description repeats the title.
- [ ] With more roles than one `choice` allows, every live role appears in exactly one pass-1 question, and pass 2 offers each chunk's top five plus `none_fit`.
- [ ] A pass-2 `none_fit`, or a top probability below the floor, writes no classification and records the posting as deferred with its reason.
- [ ] A written classification's stored candidates include the full pass-2 role distribution, runner-up included.

**Posting atomicity and deferral**
- [ ] When any Jev call for a posting fails after retries, no classification is written for that posting, it is recorded as failed, and other postings in the run still write.
- [ ] A 429 or 529 is retried after the `retry-after` interval; exhausting retries marks the posting failed.
- [ ] A posting deferred under the current `prompt_version` is not selected by the next run. The same posting is selectable after the version is bumped, and the MCP enrichment preview still returns it as unclassified.

**Label rules**
- [ ] A `noul` label at the floor is kept and one just below is dropped. A label at half the top label's probability is kept and one just below half is dropped. Labels past the cap are dropped lowest first.
- [ ] A posting where no specialization or model-judged skill survives still writes a classification with those arrays empty.
- [ ] A candidate at the recording floor is stored and one just below is not.
- [ ] Ambiguous skill names such as Go, R, C, C++ and C# match only under their context rules, with a fixture that must match and one that must not.

**Seniority core**
- [ ] The core reproduces v9's documented examples, including a level word used as a level in title and in body, "senior stakeholders" not counted as a level, and `unknown` with no note when no phrase qualifies.
- [ ] The `infer_seniority` example call above returns the documented shape against fixture postings. `cmd/classify` produces the same seniority and note for the same postings.
- [ ] `infer_seniority` rejects an empty selection, more than 100 ids, and an id from another company, each with a structured error and no results.

**Re-derivation**
- [ ] Re-derive over a prior run makes zero Jev calls. Its labels equal the new thresholds applied to that run's stored candidates, it writes under the bumped `prompt_version`, and its run record names the source run.

**Writes, provenance, read model**
- [ ] A written role carries exactly its existing dimensions, and a run adds no rows to `canonical_role_dimensions`.
- [ ] Against the test database on the action role, a classification persists each label's instrument, its candidates, its run reference, the pinned `prompt_version`, and the model id from the Jev response.
- [ ] A `save_enrichment` call without instruments, candidates, or run id, the shape the MCP skill sends, still succeeds.
- [ ] A `save_enrichment` call with zero roles returns a structured error and writes nothing, both through the SQL function directly and through the MCP tool; the same call with one role succeeds.
- [ ] Re-running a posting appends a new classification and leaves the earlier row unchanged.
- [ ] A run's outcome rows account for every selected posting, and its closing counts of written, deferred, and failed match them.
- [ ] Dry-run leaves every classification, candidate, and run table unchanged.
- [ ] A changed seed-table hash under an unchanged `prompt_version` prints a warning, and an unchanged hash prints none.
- [ ] `latest_classifications` returns `prompt_version`, `model`, and the run reference for both a `classify-v` row and a legacy row, and `pnpm test:db` passes.
- [ ] The migration's own down and up files round-trip on the test database, and `action_role.sql` re-runs cleanly and grants the new run functions.

### Manual
- [ ] Owner-adjudicated gold set of about 100 fresh postings exists as JSONL, labelled under v9 with titles hidden.
- [ ] A probe dry-run over about 150 postings, sampled under the selection core's per-company cap, reports role top-1 and top-3 accuracy against the gold set, accuracy by confidence bucket, the role flip rate across two runs, per-instrument precision and recall for skills and specializations, the co-fire rate of near-synonym pairs flagged by the existing slug and name similarity, precision of non-`unknown` seniority, and cost per 1,000 postings. It passes every kill criterion.
- [ ] A description arm writes role descriptions from one half of the probe sample and is compared with name-only on the other half. The owner decides whether a description column earns a follow-up.
- [ ] In a Claude and a Codex session, the batch-enrich skill calls `infer_seniority` and saves its seniority and note under the bumped skill pin.
- [ ] A pilot live run over about 40 postings shows instruments, candidates, and run references in the read model, run counts that match, and a cost within the probe's estimate. The owner runs it (`developer-guide.md` §2 Cost map).
- [ ] After the owner-run backfill, every posting first seen in the window has a `classify-v` latest classification or a deferred or failed outcome in the backfill run.

## Path

- **Reuse.**
  - `selection.Select` and its criteria, extended with the first-seen window and the deferral exclusion.
  - `boilerplate.NewDBLoader` and `boilerplate.CleanSelected`, imported rather than shelling out to `bin/strip-boilerplate`.
  - `classify.LoadTaxonomy` and `classify.Validate`.
  - `db.Queries.SaveEnrichment`.
- **Extract.** `validateProvenance`, `buildPayload` and the result decoding from `cmd/mcp/save_enrichment.go` (539 lines) move to an `internal/` package in their own behavior-preserving commit.
- **Precedents.**
  - `strip_boilerplate` for `infer_seniority`.
  - `title_seniority_seeds` (000036) for the seed table.
  - `fetch_runs` (000005) for the run table.
  - The per-function grant blocks in `action_role.sql`.
- **Deleted with the old runner.** `codex_exec.go`, `prompt.go`, `batched_response.schema.json`, `writeback.go`. The progress and report files may seed the new ones. Update the references in `README.md`, `.gitignore`, `.claude/settings.json`, `.codex/config.toml`, and the preflight, data-audit, and implement-task skills. Durable docs update at promotion.
- **Skill edits.** Both `batch-enrich` skills replace their seniority ladder with a call to `infer_seniority` and bump the shared pin together.
- **Shape and rival.** Chosen: deterministic code plus Jev, per dimension. Strongest rival: a cheap hosted LLM picking from a closed enum. It is simpler, gives up calibrated per-label probabilities, and is the fallback if the kill criteria fire.
- **First slice.** The Jev client, role pass 1 and pass 2, title masking, and dry-run over about 10 postings. This checks the request shape, whether the context budget is 32k or 64k (`research.md` §Jev), whether the dated model id is accepted, and ~250-way role accuracy, the riskiest assumption.
- **Go idioms.**
  - Bounded concurrency with `errgroup` and `SetLimit`; writes serialize on `save_enrichment`'s advisory lock anyway.
  - Wrap errors with `%w`.
  - Declare the decider interface where it is used, not where it is implemented.
- **Seed authoring.** Generate lexical aliases from skill names, hand-write context rules for ambiguous names, and choose the non-lexical `noul` head by link mass (`research.md` §Skill coverage).

## Open questions

- Alpha decisions endpoint or `/api/v1/systemone`. — **delegated**
- Floor, relative ratio, cap, and recording-floor values per dimension, set from the probe. — **delegated**

## Boundary inventory

| Name | Go struct field | JSON key | SQL column |
|---|---|---|---|
| Posting id | `PostingID` | `"posting_id"` | `classifications.job_posting_id` |
| Label instrument | `Method` | `"method"`: `"lexical"`, `"jev"`, `"rule"` | `method` on each join table and the candidate table |
| Candidate probability | `Probability` | `"candidates"[].probability` | `probability` on the candidate table |
| Run reference | `RunID` (`*int64`) | top-level `"run_id"` | `classifications.run_id` |
| Prompt version | `PromptVersion` | `p_prompt_version` argument | `classifications.prompt_version` |
| Model | `Model` | `p_model` argument, from the Jev response `"model"` | `classifications.model` |
| Seniority tag | `Tag` | `"tag"` | none; carried in the note line in `classifications.notes` |
| No-fit option | `noneFit` | `"none_fit"` option key | outcome reason in the run outcome table |
