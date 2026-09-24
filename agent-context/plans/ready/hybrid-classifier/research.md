# hybrid-classifier: Research

> **Read this when:** you need the derivation behind a Decision in `index.md`, or a number to re-measure.
> **Key invariant:** numbers here are dated snapshots (2026-09-23, commit 195f8c9). Re-measure before relying on one.
> **Related:** `research/enrichment-tool-design-inputs.md` (the constraints this brief answers), `index.md`.

---

## Jev over OpenRouter

Sources: OpenRouter endpoints JSON for `typesafe/jev-1.13`, docs.typesafe.ai (`/models`, `/primitives/*`, `/confidence`, `/model-jaggedness/jev-1.13`, cookbooks), OpenRouter Jev guide and tutorial.

| Fact | Value | Confidence |
|---|---|---|
| Endpoint | `POST https://openrouter.ai/api/alpha/decisions`; `POST /api/v1/systemone` is the TypeSafe-SDK-compatible alternative. Not `/chat/completions`. | High |
| Request | `{model, state, questions}`. `state` is a string, object, or array. `questions` maps an id to `{type, instructions, criteria?}`. Question ids are not sent to the model. | High |
| `choice` | Single-select, at most 255 options. Returns `choice`, `probabilities` (sums to 1), `confidence`. No built-in abstain; add a `none_fit` option. | High |
| `noul` | Yes/no. Returns P(yes) only. Absolute, not relative: all can be low; nothing stops many being high. | High |
| Criteria | Each option maps to a string, object, array, or null. Keys and descriptions both reach the model. | High |
| Billing | State ingested once per request; each question adds its own tokens. Prompt $0.042/M; output free. | High |
| Context | TypeSafe: 32k for state + longest question, 64k for state + all questions. OpenRouter lists 32k. **Conflict: verify in the first slice.** | Medium |
| Determinism | Not guaranteed. Cookbook: 11 of 13 answers identical over 5 runs; a choice test flipped 2 of 8. No seed or temperature. | High |
| Limits | TypeSafe native: 250k tokens/s, 1,200 req/min. Errors 429 and 529; honor `retry-after`. OpenRouter per-call question cap undocumented. | Medium |
| Model id | Request slug `typesafe/jev-1.13`; response reports the dated id (`typesafe/jev-1.13-20260917`). Docs warn aliases move. | High |
| Large taxonomies | Docs: chain `choice` questions level by level, or narrow in two passes. Flat example tops out near 75 classes. No public evidence at ~250-way. | High |

## Taxonomy and data snapshot

| Measure | Value |
|---|---|
| Roles / specializations / skills / dimensions | 291 / 410 / 2,586 / 8 |
| Mean labels per classification | roles 1.03, specializations 2.2, skills 5.9 |
| Roles used once; roles with ≥2 uses | 50; 238 |
| Role minting rate | 45 new the week of 09-07, 28 the week of 09-21 |
| Specializations with ≥3 uses | 307; the rest carry 130 of 11,918 links |
| Skills used once | 1,113 |
| Retired slugs still live in a taxonomy table | 0 |
| `batch-enrich-v9` classifications stored | 0. Stored rows are v2–v6 plus `mcp-save-enrichment-v1`. |
| Classifications with zero roles | 2. Neither `classify.Validate` nor the SQL function enforces a minimum. |
| Taxonomy description columns | None. `canonical_roles`, `specializations`, `skills` hold id, slug, name, created_at. |

| Postings first seen within three months, with a description | 11,089 of 17,257 total; the corpus starts 2026-05-15. Backfill ≈ $11. |
| `latest_classifications` columns before this brief | job_posting_id, classification_id, seniority, classified_at. No lineage. |

Seniority values: intern, junior, mid, senior, staff, principal, lead, director, unknown.

## Skill coverage by instrument

The top 220 skills by link mass, hand-bucketed; they cover 79% of 34,112 links.

| Bucket | Terms | Share of links | Instrument |
|---|---|---|---|
| Named tech, languages, platforms | 52 | 20% | Lexical dictionary |
| Soft competencies | 38 | 24% | Jev `noul` |
| Domain methods (financial-modeling, gtm, compliance, observability) | 131 | 35% | Jev `noul` |
| Tail, rank >220 | 2,332 | 21% | Lexical if the name is distinctive; otherwise unreachable |

Lexical probe on 400 postings against stored labels: python P 88% / R 84%, kubernetes 97% / 78%, typescript 88% / 92%. Most misses are labels the LLM inferred without a phrase, which v9 forbids. Lexical is useless for soft skills: "communicat" matches 294 of 400 postings; 26 carry the label. Lexical recall on specializations: 25%.

## Seniority

v9's ladder is not a closed rule set. Step 1 counts a level word only when it acts as a level, in title or body; Step 2's first two classes and uncovered compounds are open judgment. In the latest descriptions of 1,000 postings, 615 contain a level word: lead 369, senior 300, director 218. A word match on the body fires on most postings.

`title_seniority_seeds` is a different vocabulary: it adds associate, chief, and distinguished, lacks bare Graduate and bare Mid, keeps the highest rank where v9 takes a band's lower bound, and matches `staff` in "Chief of Staff", which v9 says carries no level. Its patterns use lookaround that Go's RE2 rejects.

Computing seniority inside the save path was also costly: each save reloads and strips its company's whole corpus, up to 1,458 postings (6.3 MB) against the MCP read pool's 5-second timeout. Tool-local extraction plus a Jev check avoids both the contract change and that cost.

## Title leakage

In a 300-posting sample, 24% of descriptions contain the full title verbatim and 53% contain the reference role's name. 46% of reference role names appear in the title. `forward-deployed-engineer` is itself a canonical role. An unmasked role question measures the title, and the title-vs-role divergence collapses by construction.

## Cost and wall-clock

Per posting: state ~1.3k tokens (median raw description 5.6k chars, less boilerplate), role pass 1 ~6k, pass 2 ~1.6k including the second state, specialization nouls ~8k (410 × ~20), skill nouls ~6k (~300 × ~20). Total ~23k tokens. Seniority adds one `noul` per candidate level phrase, a few per posting at ~20 tokens each.

- 1,000 postings ≈ 23M tokens × $0.042/M ≈ **$0.97**, plus OpenRouter's credit fee.
- Full corpus (~20k) ≈ **$19**.
- Wall-clock ≈ 5–6 minutes per 1,000 at concurrency 8. `save_enrichment` serializes on its advisory lock at ~0.1 s per write.
- Deterministic parts run in milliseconds.

## Local runtime findings

Owner hardware: Intel i9-9980HK, 32 GB, no usable GPU.

| Runtime | Status on Intel macOS | Confidence |
|---|---|---|
| PyTorch | Last x86_64 wheel is 2.2.2. Current `transformers` needs torch ≥2.5, so sentence-transformers is frozen on old pins. | High |
| onnxruntime | Last x86_64 macOS wheel 1.23.2; later releases arm64 only. | High |
| Ollama | Ships darwin builds, x86 CPU-only, macOS 14+. Plain HTTP from Go, no cgo. | High |
| hugot (pure Go) | Works; scoped to MiniLM-sized models. | Medium |

A 137M-parameter embedding model at 512 tokens runs ~1.5 s per posting on this CPU: ~25 minutes per 1,000, ~8 hours for a corpus backfill. MiniLM-class is ~4× faster. Medium confidence.

## Alternatives rejected

| Alternative | Why not |
|---|---|
| Embedding shortlist, then Jev `choice` for role | General embeddings cluster by topic, not function; a 1k-token posting against a three-word label is embeddings' weakest use. Backfill cost above. Embeddings belong to taxonomy hygiene and title-fit. |
| Prune roles to fit 255 | Leaves 14 slots of headroom against 28–45 new roles a week, and silently sets the taxonomy. |
| Jev `noul` for every skill | 2,586 questions overflow the budget; a 1% false-yes rate adds ~26 wrong skills against a true 5.9. |
| kNN or linear model on stored labels | Inherits title-contaminated v2–v6 labels; weak on singleton roles; retrains on every mint. A distillation step after Jev, not an alternative. |
| Local 7B LLM | Impractically slow on this CPU. |
| Unattended v9 with minting over a hosted LLM via OpenRouter | Mint rate: 78 roles in 14 days even under supervision, followed by cleanup migrations 000031, 000037, 000038, 000040. Unattended minting multiplies that. |
| Pick-only v9 with closed enums over subscription `codex exec` | No per-posting cost, and the transport exists in `cmd/batch-enrich`. Loses only on stored per-label probabilities and offline re-derivation. The likelier fallback if the kill criteria fire. |
| Cheap hosted LLM with a closed enum (~$0.40/1k) | The fallback if Jev fails the kill criteria. Gives up calibrated per-label scores. |
| Zero-shot NLI models | One forward pass per label; too slow and coarse at this label count. |

## Direction review, round 1

`/validate-plan` returned *Under-scoped*: a second classifier entered `latest_classifications` with no read-model story; write-time thresholds made re-tuning a paid, non-deterministic re-run; single-role and duplicated seniority rules broke stated commitments unnamed; deferred postings were re-selected and re-paid; the implemented `codex-native-batch-enrichment` plan still sat in `ready/`. Owner resolutions: lineage in the view plus a three-month backfill; probabilities stored with offline re-derivation; one assigned role with the blend measured from stored pass-2 probabilities; one seniority core shared through MCP.

## Direction review, round 2

`/validate-plan` returned *Under-scoped*, direction unchanged. Skill reach capped at the seed table reads as zero in the measure engine, whose classified denominator counts any latest classification; deferrals to the skills keep the window mixed-lineage, so the single-classifier claim was false; seniority evidence stayed in `notes` without a warrant. A read-only query found 1,132 of 12,260 postings first seen in the last three months classified, the last on 2026-09-21. Owner resolutions: read-model lineage, engine reach, and the backfill move to a follow-up brief, `lineage-aware-measures`, and live runs stop at the pilot until it lands; seniority evidence stays in `notes`; a two-phase owner gate at the probe.

## Direction review, round 3

`/validate-plan` returned *Direction sound*, conditional on the round-2 split, and recommended `/review-brief` before sign-off. Seven findings, all adopted: seniority relayed through an agent collapsed a deterministic value into an agent claim, so it moved into the save path and the planned `infer_seniority` tool was dropped; a core change needs a pin rule spanning both lineages; Step-1 level words were a third vocabulary copy; Jev's nondeterminism breaks the parked dedup draft's premise, so answers are shared per distinct state; re-derive could supersede newer rows; the minting claim held for roles only; the subscription `codex exec` rival was unnamed.

Premise found while applying: `title_seniority_seeds` patterns use lookbehind and lookahead (for example `(?<!thought\s)leader`), which Go's RE2 rejects. Its 14 ranks include associate, manager, head, vp, and chief, so a rank-to-classifier mapping is required.

## Ordering pins

Acceptance rows cite these by id.

| Id | Scenario | Ordering | Expected outcome |
|---|---|---|---|
| P1 | Failed posting | Run 1 records the posting failed after Jev retries are exhausted; run 2 starts under the same `prompt_version`. | Run 2 selects the posting again. Only deferrals are excluded. |
| P3 | Deferral, then text change | The posting is deferred under `classify-v3`; a new snapshot changes its description; the next run starts under `classify-v3`. | The posting stays excluded. The exclusion is keyed on lineage and version, not text. |
| P4 | Dry-run, then live | A dry-run defers the posting; a live run starts under the same `prompt_version`. | The live run selects the posting and makes its own Jev requests. A dry-run writes no run row, and answers are shared only within a run. |
| P5 | Same posting, two runs | Two runs classify an unchanged posting. | Each run sends its own Jev requests; no answer carries across runs. The flip-rate measure depends on this. |
| P6 | Same text, two companies | Two postings at different companies share a raw description; their cleaned, title-masked states differ. | Two Jev requests. When the requests are identical after all whitespace is removed, one request. |
| P7 | Shared answer fails | Two postings share one request key; that request fails after retries. | Both postings are recorded failed and neither is written. |
| P8 | Re-derive race | The source run recorded the posting's latest classification at selection; a skill save for the posting commits after that; re-derive then saves. | The save's expected-latest check fails under the lock; re-derive writes nothing and records the posting skipped. |
| P9 | Re-derive under a raised role floor | The source run wrote the posting; the new role floor is above its stored top role probability. | Re-derive writes nothing, records the posting deferred with its reason, and the source-run row stays latest. |
| P10 | Re-derive over a source-run deferral | The source run deferred the posting and stored its candidates; re-derive lowers the role floor below the stored top probability. | Re-derive writes the posting, provided its latest classification is still the one recorded at selection. |
| P12 | Seed table change under one version | Run 1 records seed hash H1; the skill seed table changes; run 2 starts under the same `prompt_version`. | Run 2 warns, proceeds, and records H2. The first run under a new `prompt_version` prints no warning. |
| P13 | Migration over existing rows | The migration applies to a database holding zero-role classifications and legacy join rows. | Zero-role rows are unchanged, legacy join rows read a null instrument, and legacy classifications read a null run reference. |
| P14 | Role minted mid-run | A skill save mints a role after a classify run reads its option sets. | The role is not offered in that run and is offered from the next run on. |

## Detail review

`/review-brief`: three lenses. The premise lens found the seniority ladder could not be ported as rules, and the save function's near-duplicate drop contradicting stored candidates. The rows lens added ordering pins and rows. The subtract lens proposed dropping re-derive and splitting seniority out. Owner resolutions: seniority becomes tool-local candidate extraction with a Jev check, and the skills keep v9; re-derive stays as the first consumer of stored candidates, guarded by an expected-latest check under the save lock and copying rule and lexical labels from the source row; the tool pre-dedups near-duplicate labels; sixteen smaller items settled as listed in Decisions.

## Direction review, round 4

`/validate-plan` returned *Direction sound*. Owner resolutions: candidates hang off the run outcome, so deferred postings keep their evidence and re-derive moves a floor either way; the seed table is generated per install; seed and candidate references follow `profile-and-pins` term repair; read-at refreshed past the profile-led direction; the old runner survives until after the pilot; a dated Jev model change does not bump `prompt_version`.

## Review trail

An independent reviewer and the drafting session converged over three rounds. Positions changed: the embedding shortlist was dropped; taxonomy pruning was dropped for a two-pass tournament; the skill plan grew a Jev arm when domain methods proved 35% of link mass; `prompt_version` stayed a hand-bumped constant per `developer-guide.md` §6.2 instead of a hash that encodes the model. The reviewer's conditions: split the probe so description-writing and scoring use disjoint halves; every classification row carries its run id.
