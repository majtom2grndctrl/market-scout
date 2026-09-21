---
name: data-audit
description: Audit Market Scout classification quality and taxonomy drift without changing data. Use during active collection or after changing classification, boilerplate stripping, or company discovery.
---

# Data Audit

Audit agent-written enrichment data. Read-only: never reclassify, merge, or write rows.

## Start

Read `agent-context/lib/developer-guide.md` §6.2. Confirm the project MCP server exposes read-only query access. If it is unavailable, stop; do not substitute direct database writes or ad-hoc mutations.

Defaults: sample 20 current classifications from the last 14 days. Accept `sample:N` and `since:YYYY-MM-DD` to override.

## Process

1. Read the canonical current-classification query before sampling. It selects the latest classification per posting.
2. Sample across companies and ATS sources. Include emergent taxonomy entries when the window contains any.
3. Give independent reviewer agents small, disjoint sets of postings. Inline each posting description and its classification.
4. Require one verdict per posting: `sound`, `wrong-role`, `junk-summary`, or `suspect-taxonomy`. Each verdict cites one sentence from the description. No praise.
5. Check drift yourself:
   - failures in `agent-output/batch-enrich/failures.jsonl`;
   - emergent taxonomy growth and cross-table slug collisions;
   - quality split by `prompt_version` when the sample spans versions. Never infer a version from a model. Pre-2026-09-21 cohorts that developer-guide §6.2 marks ambiguous stay unresolved — do not fold them into a neighbouring version.
6. Report the sound rate with numerator and denominator. Group findings by pattern, then list drift flags and suggested follow-ups.

## Rules

- Keep each reviewer's input inline. Do not rely on paths being read.
- Verify identifier and schema claims from files opened this session.
- Recommendations may propose a targeted `--force` run, a stripping fix, or a taxonomy merge. Do not apply them.
- Record sample size, window, sound rate, and prompt-version mix in the report. No persistent audit ledger exists.
