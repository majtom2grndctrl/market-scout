# hybrid-classifier: gold labeling

> **Read this when:** labeling the probe's gold set, or resuming that step.
> **Key invariant:** labelers never see a title, and nothing they do writes to the database.
> **Related:** `index.md` (Decisions: Gold set before the probe), `plan.md` (Task 7).

---

## Procedure

Every path below is under `apps/tools/agent-output/classify/probe/`, which is gitignored.

1. `go run ./cmd/classify sample` wrote `sample.jsonl` and `labeler-input.jsonl`: 150 postings, title-masked. Do not re-run it; a re-run can select different postings.
2. `go run ./cmd/classify taxonomy-export` wrote `roles.tsv`, `specializations.tsv`, `skills.tsv`.
3. `labeler-input.jsonl` is split into `chunks/chunk-NN.jsonl`, 10 postings each.
4. Two labelers label every chunk independently: Sonnet 5 as `sonnet`, Opus 5.5 as `opus`. Each writes `labels/<labeler>-chunk-NN.jsonl`.
5. `go run ./cmd/classify gold-merge` writes `disagreements.json`: postings where the labelers split on role or seniority.
6. A third labeler, Fable, labels the split postings blind, from `tiebreak/chunk-NN.jsonl` to `tiebreak/fable-chunk-NN.jsonl`, under the same instructions. It never reads `labels/` or `disagreements.json`. The chunk files are concatenated into `tiebreak/fable.jsonl`.
7. `gold-merge --tiebreak tiebreak/fable.jsonl` settles every split where Fable agrees with one labeler, and rewrites `disagreements.json` with the rest: the three-way splits.
8. The owner settles those. Decisions are saved as `decisions.json`, a JSON array of `{posting_id, role, role_any, seniority}`: `role` is a slug, or `none_fit` when no live role fits; `role_any` lists further roles that are equally right. A decision overrides the tie-break.
9. `gold-merge --tiebreak tiebreak/fable.jsonl --decisions decisions.json` writes `gold.jsonl`.

## Labeler instructions

You are labeling postings for a gold set, the yardstick a new classifier is measured against. Label under the v9 contract with the title hidden.

**Read first:** `.claude/skills/batch-enrich/SKILL.md` §7 "Classification discipline", lines 329–500. It is the contract. Read it fully. Ignore every instruction about `save_enrichment`, MCP tools, selection, and dispatch; they do not apply here.

**Input.** Your chunk file holds one JSON object per line: `posting_id` and `masked_description`. The title was removed and each occurrence replaced with `[—]`. Do not try to recover the title from any other source.

**Taxonomy.** Read `roles.tsv` and `specializations.tsv` in full; each line is `slug<TAB>name`. `skills.tsv` is long: search it with `grep -i` for each skill you consider. Do not read it whole.

**Differences from v9:**

- **Role:** exactly one, the live role in `roles.tsv` that best fits the job the posting hires for. If v9 would give two, give the primary. If no live role genuinely fits, the case where v9 would mint, set `"role": ""`, `"role_uncovered": true`, and `"proposed_role_name"` to what you would have minted. Do not stretch a poor fit to avoid this.
- **Specializations and skills:** existing slugs only. Where v9 would mint a new term, leave it out. v9's grounding rules still hold: a skill is included only when the posting asks for it.
- **Seniority:** v9's ladder as written, Steps 1 through 3, reading the body only, since the title is hidden. Put the evidence line in `seniority_note` in v9's format, `seniority[<tag>]: "<verbatim phrase>"`, or leave it empty for `unknown`.
- **Nothing is saved.** No MCP calls, no database queries, no `save_enrichment`. Write your output file and nothing else. Do not read other labelers' files.

**Output.** Write `labels/<labeler>-chunk-NN.jsonl`, one line per input posting, in input order:

```json
{"posting_id": 105665, "labeler": "opus", "role": "warehouse-associate", "role_uncovered": false, "proposed_role_name": "", "specializations": ["logistics-supply-chain"], "skills": ["inventory-management"], "seniority": "unknown", "seniority_note": "", "role_rationale": "One sentence: which duties decide the role."}
```

Before finishing, check that every slug you used appears exactly as a first column in its TSV (`cut -f1 roles.tsv | grep -x <slug>`), and that the file has exactly as many lines as your chunk.
