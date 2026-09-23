# hybrid-classifier

Brief · resumable · reads: `agent-context/lib/project.md` §Settled architecture, §Evidence trust tiers · `agent-context/lib/developer-guide.md` §2 Cost map, §2 Test database, §6.2 · `research/enrichment-tool-design-inputs.md` · read at 2bde04f

## Problem

Owner-requested capability. The Go enricher, `cmd/batch-enrich`, shells out to LLM CLIs and writes taxonomy and classification rows directly under owner credentials, a second write route that skips every gate `mcp.save_enrichment` enforces; it stays unused until rewritten. The owner wants an unattended classifier that is cheap per posting, uses the right instrument for each dimension, and keeps the evidence behind each label. When done, an operator runs one Go binary over a selected cohort. It writes one append-only classification per posting through `mcp.save_enrichment`, or writes nothing and records why. Every label records its instrument, and every model judgment keeps its probabilities, so thresholds can be re-tuned from stored evidence without a new model call.

## Decisions

- **New binary `cmd/classify` replaces `cmd/batch-enrich`.** None of the LLM contract survives: prompt, response schema, hint retries, direct writeback. The old runner is removed after the pilot, so its `codex exec` transport survives as the fallback until the tool has proven itself on live writes. `codex-native-batch-enrichment` excluded rewriting the runner from its own scope; that plan is implemented and sets no standing rule.
- **OpenRouter serves an automation path too.** Diverges from `project.md` Settled architecture, whose Model access row reads "interactive surfaces only" and whose OpenRouter paragraph says bulk classification needs no OpenRouter path; both change at promotion. Cost is in `research.md` §Cost. The Codex skill stays the interactive path and the only minting path.
- **One instrument per dimension; quotable evidence gets code, contextual judgment gets the model.** This is the deterministic-vs-agent-judgment axis, and each label records which side produced it.

  | Dimension | Instrument | Options offered |
  |---|---|---|
  | Seniority | Code extracts candidate level phrases from title and body; a Jev `noul` per candidate judges whether the word acts as a level; v9's ladder picks the value | Closed enum |
  | Skills | Lexical match from a skill seed table, plus Jev `noul` for seed-listed non-lexical skills | Only skills the seed table lists |
  | Specializations | Jev `noul` per specialization | Every live specialization |
  | Role | Jev two-pass `choice` | Every live role |

- **Seniority is tool-local; the skills keep v9.** v9's ladder turns on judgment a word match cannot make: a level word counts only when it acts as a level, and most descriptions carry one (`research.md` §Seniority). Candidate extraction keeps the verbatim phrase; the Jev check supplies the judgment. The two lineages hold different seniority instruments, recorded by lineage. The tool's level-word list starts from v9's Step 1 and can drift from it; a v9 change to Step 1 prompts a `classify-v` review, not an automatic bump.
- **Seniority keeps v9's evidence format.** Notes carry `seniority[<tag>]: "<verbatim phrase>"`; `unknown` carries none. Diverges from `research/enrichment-tool-design-inputs.md` §Provenance and lineage, which places a seniority source column in this tool's output: no consumer parses the tag yet, so the column waits for its first reader.
- **Pick-only.** The tool never mints. Option sets are read from the live taxonomy at run start. A skill absent from the seed table is unreachable until seeded; newly minted skills are too. Non-lexical skills outside the `noul` head are unreachable by design, so skill comparisons across classifiers use the vocabulary both can reach.
- **The skill seed table owns skill reach, and each install generates its own.** A generator builds seed rows from the install's live skills: lexical aliases from skill names, and the `noul` head chosen by that install's link mass. Only the context rules that disambiguate short names such as Go, R, and C ship as curated data, applied where a matching skill exists. Each install grows its own taxonomy (`project.md` §Settled architecture), so a hand-written seed would reach no skills on a fork.
- **New term references follow term repair.** The seed table and the candidate table reference taxonomy terms by foreign key. Merge and retire must carry every reference a term has (`plans/drafts/profile-and-pins`), so whichever of the two briefs lands second extends the other's repair functions to cover its references. Re-derive over a candidate naming a term retired since the source run fails that posting, as a mid-run change does.
- **Role reads the description with the title masked.** Masking covers the full title and the title stripped of level words. The research measures how far a title strays from its role (`research/enrichment-tool-design-inputs.md` §Opportunities, Title fit); an unmasked question measures the title (`research.md` §Title leakage). Seniority does read the title, as v9's Step 1 does.
- **Role is a tournament over the full taxonomy, at every size.** Pass 1 splits all roles into stable chunks under Jev's option cap, each with `none_fit`. Pass 2 is one `choice` over each chunk's leaders plus `none_fit`. No pruning: a usage floor would silently set the taxonomy. Options are offered by name only.
- **One assigned role; a blend is measured, not labelled.** Diverges from v9's "blended roles are first-class". Skills have no mapping to roles, so a skill mix cannot say which roles a posting blends. The stored pass-2 distribution can: the gap between the top two probabilities measures the blend directly.
- **One Jev answer per distinct request within a run.** Jev is not deterministic, so identical postings asked twice can split. A request is identified by its state and questions with all whitespace removed, the text identity `plans/drafts/work-unit-dedup-edges` proposed; postings sharing it share the answer. That draft's premise that a non-LLM tool gives identical answers for identical text does not hold for Jev.
- **All or nothing per posting.** If pass 2 returns `none_fit`, the top role is below the floor, or any Jev call for the posting fails after retries, nothing is written. A zero-role row would blank the posting's older role in `latest_classifications`.
- **`mcp.save_enrichment` rejects a classification with zero roles.** This guards every writer, including the MCP skills. Existing zero-role rows stay; storage is append-only.
- **Run outcomes and deferral.** Each run records a per-posting outcome: written, deferred with reason, skipped, or failed. The tool does not re-select a posting it deferred under the current `prompt_version`, unless forced. The skills' selection is unchanged, so they pick deferred postings up as unclassified, and the minting path receives every posting whose role the taxonomy does not cover. A posting with a covered role and a new skill or specialization is classified and never reaches a minting path.
- **Runs end in a known state.** A run with no postings still records a run. A run that stops early stays `in_progress`, as `fetch_runs` does; outcomes already recorded stand. Two writers on one posting are accepted: saves serialize, storage is append-only, and the later write is latest.
- **Mid-run change fails the posting, not the run.** A term retired or merged between run start and write fails that posting, and the next run retries it with fresh options. A posting is classified from the snapshot read at selection. `model` is the id each response reports.
- **Multi-label rule for `noul` dimensions: floor, relative, cap.** A label is kept only when its probability clears the floor, reaches at least half the top label's, and ranks within the cap. Of two kept labels that `save_enrichment` treats as near-duplicates, the tool keeps the more probable, so the save function has nothing to drop. Values are tuned for precision on the gold set, not for historical label density. Zero surviving labels is a valid result.
- **Probabilities are stored; labels are applied at write time.** Every Jev candidate above a low recording floor is stored with its probability against the posting's run outcome, deferred postings included, with the full pass-2 role distribution. Join tables keep meaning "assigned label", so no existing view changes meaning. A probability is stored once, in the candidate table.
- **Re-derive is the first consumer of stored candidates.** It re-applies new thresholds to a prior run's Jev candidates and writes new classifications under a bumped `prompt_version`, with no Jev call. Because deferred postings keep their candidates, a floor can move either way: raising it defers postings the source run wrote, and lowering it writes postings the source run deferred. Rule and lexical labels carry no threshold and are taken from the source run's outcome. It writes only where the posting's latest classification is still the one recorded when the source run selected it, checked under the save lock; otherwise it records the posting skipped. Candidates below the recording floor are gone, so a floor raised above it cannot be re-derived.
- **A role is written with its existing dimension set.** `save_enrichment` requires dimensions; echoing the current set adds no `canonical_role_dimensions` rows, so the ratchet (`research/enrichment-tool-design-inputs.md` §Taxonomy interaction) does not grow.
- **Writes go only through `mcp.save_enrichment` on the action role.** The Go-side checks the MCP handler runs move to a package both binaries import. Labels saved through the MCP tool record the instrument `agent`; agents never send instruments, candidates, run ids, or the expected-latest precondition.
- **Schema, additive.**
  - An instrument column on the role, specialization, and skill join tables. Legacy rows stay null.
  - A candidate table of term, instrument, and probability per run outcome.
  - A classification run table, a per-posting run outcome table recording the posting's latest classification at selection, and a nullable run reference on `classifications`.
  - A skill seed table holding reach, aliases, and context rules.

  Run rows are written through new approved `mcp` functions, because the action role holds only per-function EXECUTE (`action_role.sql`). The payload extension keeps the external `mcp.save_enrichment(jsonb, text, text)` signature. Undo cost: the down migration drops the tables and columns, discarding stored probabilities and run history.
- **Live runs stop at the pilot.** Any larger live run, the recent-window backfill included, waits for the follow-up brief `lineage-aware-measures`. A `classify-v` row that becomes a posting's latest hides skills the tool cannot reach, and today's measure engine reads that absence as zero (`project.md` §Settled architecture: absent is not zero). The window stays mixed-lineage by design, because deferred postings go to the skills.
- **Two phases with an owner gate.**
  - Phase 1 writes nothing: the Jev client, the role tournament, seniority, label rules, dry-run with the seed table loaded from a file, the gold set, and the probe.
  - Phase 2 starts when the owner accepts the probe: migrations, the `save_enrichment` extension, writes, run records, re-derive, the pilot, and then removal of the old runner.
- **Provenance.** `prompt_version` is a new lineage, `classify-v<n>`, pinned once in Go and bumped by hand for any change to rules, thresholds, or question wording (`developer-guide.md` §6.2: never encode the model). Each run records its thresholds and the hashes of the role and specialization option sets, the seed table, and the question wording. The tool warns at startup when the seed hash differs from the last run under the same `prompt_version`, and proceeds. A new dated Jev model does not bump `prompt_version`, so deferrals under the current version stay excluded across a model change; `model` records the change.
- **Transport: OpenRouter's decisions endpoint behind an interface declared in the consuming package.** The interface lets tests use a fake. The key comes from `AI_SERVICE_KEY`, which nothing else reads today.
- **Dry-run is the probe.** A dry-run mode runs the real pipeline, writes no classification and no run row, and emits per-posting JSONL.
- **Gold set before the probe.** About 100 fresh postings are labelled under the v9 contract with the title hidden, through the subscription skill path. They are written to JSONL, not the database, because title-hidden v9 is not the v9 contract. The owner settles disagreements.
- **Kill criteria stop the build before any live write.** Scored name-only on the full probe sample. Stop and report to the owner if role top-1 falls more than ten points below the gold set's own agreement, if the high-confidence third of role answers is not at least ten points more accurate than the low-confidence third, or if more than 5% of role answers flip across two runs.
- **Integration tests run against `market_scout_test`**, with the action role provisioned there.
- **Non-goals.**
  - Embeddings; they belong to taxonomy hygiene and title-fit, later, via Ollama.
  - A taxonomy description column; the probe's description arm decides whether one earns a follow-up.
  - Synonym-cluster dedup; the probe's co-fire rate is its input, and taxonomy hygiene owns it.
  - The dimension-ratchet fix; `research/enrichment-tool-design-inputs.md` §Taxonomy interaction holds it open as a curation decision.
  - A seniority core shared with the skills; unifying the two seniority instruments would be its own brief.
  - Persisting required-vs-preferred skills.
  - Taxonomy growth for skills and specializations: seeding newly minted skills, and minting on postings whose role is covered. A follow-up brief, `taxonomy-growth`, owns both.
  - Lineage in `latest_classifications`, measure-engine handling of reach and lineage, and the recent-window backfill. `lineage-aware-measures` owns them, because the engine is the consumer that must act on reach.
  - Moving the existing Go integration suites to the test database.
  - An LLM fallback, which would be a new brief if the kill criteria fire.

### Agent-facing surface

`save_enrichment` gains one error an agent can hit. Every other argument, field, and error is unchanged.

```json
// call with no roles
{"posting_id": 51230,
 "provenance": {"model": "claude-haiku-4-5-20251001", "prompt_version": "batch-enrich-v9"},
 "classification": {"seniority": "unknown", "notes": ""},
 "canonical_roles": [], "specializations": [], "skills": []}
// response
{"ok": false, "classification_id": null, "posting_id": 51230, "summary": "",
 "new_taxonomy": {"canonical_roles": [], "specializations": [], "skills": []},
 "errors": [{"path": "canonical_roles", "code": "no_roles", "message": "..."}]}
```

## Acceptance

### Automated

**Role question**
- [ ] No role question's state contains the posting's title or the title with level words removed, including when the description repeats the title.
- [ ] With more roles than one `choice` allows, every live role appears in exactly one pass-1 question, and pass 2 offers each chunk's leaders plus `none_fit`. The same live role set produces the same chunks in two runs, and each option is the role's name with no slug or description. A role minted after a run reads its option sets is not offered in that run (P14).
- [ ] A pass-2 `none_fit`, or a top probability below the floor, writes no classification and records the posting as deferred with its reason.
- [ ] A written classification's run outcome stores the full pass-2 role distribution, runner-up included, and a deferred posting's run outcome stores its candidates too.

**Posting atomicity, sharing, and deferral**
- [ ] When any Jev call for a posting fails after retries, no classification is written for that posting, it is recorded as failed, and other postings in the run still write.
- [ ] A 429 or 529 is retried after the `retry-after` interval; exhausting retries marks the posting failed.
- [ ] Two postings whose requests are identical apart from whitespace produce one Jev request and identical role, specialization, and model-judged skill labels; requests that differ by a non-whitespace character produce two (P6). Two runs over an unchanged posting each send their own Jev requests (P5). When a shared request fails after retries, every posting that shares it is recorded as failed and none is written (P7).
- [ ] A posting deferred under the current `prompt_version` is not selected by the next run. The same posting is selectable after the version is bumped, and the MCP enrichment preview still returns it as unclassified. A posting recorded as failed is selected by the next run under the same `prompt_version` (P1), and a deferred posting whose description changes stays excluded under the same version (P3). A forced run selects deferred postings.
- [ ] A term retired between run start and write fails only the posting that carries it, and the next run selects that posting again.

**Label rules**
- [ ] A `noul` label at the floor is kept and one just below is dropped. A label at half the top label's probability is kept and one just below half is dropped. Labels past the cap are dropped lowest first.
- [ ] Of two kept labels that `save_enrichment` treats as near-duplicates, only the more probable is sent, and the save reports nothing dropped.
- [ ] A posting where no specialization or model-judged skill survives still writes a classification with those arrays empty.
- [ ] A candidate at the recording floor is stored and one just below is not.
- [ ] Ambiguous skill names such as Go, R, C, C++ and C# match only under their seed-table context rules, with a fixture that must match and one that must not.
- [ ] On a test database with a skill set unlike the development taxonomy, the seed generator reaches every live skill whose name is lexically distinctive, chooses the `noul` head from that database's link mass, and applies a curated context rule only where its skill exists.

**Seniority**
- [ ] Seniority reproduces v9's documented examples against a fake Jev: a level word acting as a level in title and in body, "senior stakeholders" judged not a level, "Chief of Staff" carrying no level, and `unknown` with no note when no candidate is judged a level.
- [ ] A level word the fake judges not a level produces no seniority from that phrase, and one it judges a level produces the v9 value and a note quoting the phrase verbatim.

**Re-derivation**
- [ ] Re-derive over a prior run makes zero Jev calls. Its Jev-judged labels equal the new thresholds applied to that run's stored candidates, its rule and lexical labels equal the source row's, it writes under the bumped `prompt_version`, and its run record names the source run.
- [ ] Re-derive skips and records a posting whose latest classification is not the source run's, including one saved by a skill after re-derive read it (P8). It records a posting as deferred when the new role floor exceeds its stored top probability, leaving the source row latest (P9), and writes a posting the source run deferred when a lowered floor admits its stored top role (P10). A candidate naming a term retired since the source run fails that posting.

**Writes and provenance**
- [ ] A written role carries exactly its existing dimensions, and a run adds no rows to `canonical_role_dimensions`.
- [ ] Against the test database, a `cmd/classify` run adds no rows to `canonical_roles`, `specializations`, or `skills`, and every save returns an empty `new_taxonomy`.
- [ ] Against the test database on the action role, a classification persists each label's instrument, its candidates, its run reference, the pinned `prompt_version`, and the model id from the Jev response.
- [ ] A `save_enrichment` call without instruments, candidates, or run id, the shape the MCP skill sends, still succeeds and records the instrument `agent`. The same call through the MCP tool with `run_id`, `candidates`, or a `method` added stores none of them.
- [ ] The example call above returns `no_roles` and writes nothing, both through the MCP tool and through the SQL function directly; the same call with one role succeeds.
- [ ] Re-running a posting appends a new classification and leaves the earlier row unchanged.
- [ ] A completed run's outcome rows account for every selected posting, and its closing counts of written, deferred, skipped, and failed match them. A run that selects no postings records a completed run with zero counts. A run interrupted after some writes leaves its run row `in_progress`, with the outcomes recorded before the interruption intact.
- [ ] A run record stores its thresholds and the hashes of the role option set, the specialization option set, the seed table, and the question wording. Changing one input changes only its own hash.
- [ ] Dry-run leaves every classification, candidate, and run table unchanged. A live run after a dry-run selects every posting the dry-run deferred and sends its own Jev requests for them (P4).
- [ ] Dry-run emits one JSONL line per selected posting with its outcome, its labels with instrument and probability, its seniority and note, and its request keys.
- [ ] A changed seed-table hash under an unchanged `prompt_version` prints a warning, and an unchanged hash prints none. The run still proceeds and records the new hash, and the first run under a new `prompt_version` prints no warning (P12).
- [ ] The new Go integration tests fail before connecting when their DSN names a database without the `_test` suffix, and skip when it is unset.
- [ ] The migration's own down and up files round-trip on the test database, and `action_role.sql` re-runs cleanly and grants the new run functions. Applied to a test database holding zero-role classifications and legacy join rows, the migration leaves the zero-role rows unchanged, and legacy join rows and classifications read a null instrument and a null run reference (P13).
- [ ] After the pilot (grep gate), no `cmd/batch-enrich` directory remains, and no file outside `agent-context/plans/` names `cmd/batch-enrich`, `codex_exec`, or `bin/batch-enrich`.

### Manual
- [ ] Owner-adjudicated gold set of about 100 fresh postings exists as JSONL, labelled under v9 with titles hidden.
- [ ] A probe dry-run over about 150 postings, sampled under the selection core's per-company cap, reports role top-1 and top-3 accuracy against the gold set, accuracy by confidence third, the role flip rate across two runs, per-instrument precision and recall for skills and specializations, the co-fire rate of near-synonym pairs flagged by the existing slug and name similarity, precision of non-`unknown` seniority, and cost per 1,000 postings. It passes every kill criterion.
- [ ] A description arm writes role descriptions from one half of the probe sample and is compared with name-only on the other half. The owner decides whether a description column earns a follow-up.
- [ ] No migration, `save_enrichment` change, or write path merges before the owner accepts the probe. The owner's probe acceptance is recorded in `plan.md`, and it predates the first commit that touches `internal/db/migrations/` or the save path (git-history gate).
- [ ] A pilot live run over about 40 postings persists instruments, candidates, and run references, with run counts that match and a cost within the probe's estimate. A re-derive over the pilot run writes under a bumped version. The owner runs both (`developer-guide.md` §2 Cost map).

## Path

- **Reuse.**
  - `selection.Select` and its criteria, extended with the deferral exclusion.
  - `boilerplate.NewDBLoader` and `boilerplate.CleanSelected`, imported rather than shelling out to `bin/strip-boilerplate`.
  - `classify.LoadTaxonomy` and `classify.Validate`, which also gains the zero-role check.
  - `db.Queries.SaveEnrichment`.
- **Extract.** `validateProvenance`, `buildPayload` and the result decoding from `cmd/mcp/save_enrichment.go` move to an `internal/` package in their own behavior-preserving commit.
- **Precedents.**
  - `title_seniority_seeds` (000036) for the seed table.
  - `fetch_runs` (000005) for the run table and `in_progress` state.
  - The per-function grant blocks in `action_role.sql`.
  - `apps/web/lib/db/test-dsn.ts` for the `_test` suffix guard.
- **Mechanics.**
  - Pass 1 chunks go in one request; about five leaders per chunk reach pass 2.
  - Plain `net/http` against the alpha endpoint, no SDK.
  - The re-derive precondition is an expected-latest classification id in the payload, compared under `save_enrichment`'s advisory lock.
  - The deferral exclusion is an opt-in selection criterion; `Force` lifts it.
  - Seniority candidates come from a level-word list the tool owns, seeded from v9's Step 1 and compound table.
- **Deleted with the old runner, after the pilot.** `codex_exec.go`, `prompt.go`, `batched_response.schema.json`, `writeback.go`. The progress and report files may seed the new ones. Update the references in `README.md`, `.gitignore`, `.claude/settings.json`, and the preflight, data-audit, and implement-task skills. Durable docs update at promotion.
- **Shape and rival.** Chosen: deterministic code plus Jev, per dimension. Rival: pick-only v9 over subscription `codex exec`, the likelier fallback (`research.md` §Alternatives rejected).
- **First slice.** The Jev client, role pass 1 and pass 2, title masking, and dry-run over about 10 postings. It checks the request shape, the context budget (`research.md` §Jev), whether the dated model id is accepted, and role accuracy over the full taxonomy, the riskiest assumption.
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
| Label instrument | `Method` | `"method"`: `"lexical"`, `"jev"`, `"rule"`, `"agent"` | `method` on each join table and the candidate table |
| Candidate probability | `Probability` | `"candidates"[].probability` | `probability` on the candidate table |
| Run reference | `RunID` (`*int64`) | top-level `"run_id"` | `classifications.run_id` |
| Expected latest | `ExpectLatestID` (`*int64`) | top-level `"expect_latest_classification_id"`; null means the posting had none | compared with the posting's latest `classifications.id`, as recorded on the source run outcome |
| Prompt version | `PromptVersion` | `p_prompt_version` argument | `classifications.prompt_version` |
| Model | `Model` | `p_model` argument, from the Jev response `"model"` | `classifications.model` |
| Zero-role error | `CodeNoRoles` | `"no_roles"` | none; returned in `errors[]` |
| No-fit option | `noneFit` | `"none_fit"` option key | outcome reason in the run outcome table |
