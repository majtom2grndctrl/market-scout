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
4. Require one verdict per posting: `sound`, `wrong-role`, `junk-summary`, or `suspect-taxonomy`. Each verdict cites one sentence from the description. No praise. `unknown` seniority is a correct, often-majority verdict under the current contract, not by itself grounds for `wrong-role`: `mid`/`junior` require an explicit level word and any other non-`unknown` rung requires a verbatim quoted phrase, so judge seniority against that rule — `unknown` needs no supporting phrase, any other rung does. Inline each classification's `notes`: the quote lives there as `seniority[<tag>]: "<phrase>"`, tagged with the step that produced the rung. Check the tag matches the quote's source and class: `step1-title` and `step1-body` quote a level word from the title or the body respectively; `step2-org` is `director` on a batch-enrich class (a) shape, never a reporting line; `step2-manages` is `senior` on managing individual contributors; `step2-align` is `senior` on a class (c) qualifying verb, never a collaboration verb such as "partner with". A mismatched tag, a `step2-*` tag on any rung but `senior` or `director`, or a missing tag on a non-`unknown` rung is `wrong-role` — the tag is how analyses separate stated seniority from inferred. `unknown` carries no seniority note. Rows from versions before the tag existed carry the untagged `seniority: "<phrase>"` form; judge the quote and do not flag the missing tag.
5. Check drift yourself:
   - failures in `agent-output/batch-enrich/failures.jsonl`;
   - emergent taxonomy growth and cross-table slug collisions;
   - quality split by `prompt_version` when the sample spans versions. Never infer a version from a model. Pre-2026-09-21 cohorts that developer-guide §6.2 marks ambiguous stay unresolved — do not fold them into a neighbouring version. Never compare seniority distributions across prompt versions — a version bump can deliberately reshape the contract, so a shift is the contract working, not drift; compare seniority only within one version.
6. Report the sound rate with numerator and denominator. Group findings by pattern, then list drift flags and suggested follow-ups.

## Rules

- Keep each reviewer's input inline. Do not rely on paths being read.
- Verify identifier and schema claims from files opened this session.
- Recommendations may propose a targeted `--force` run, a stripping fix, or a taxonomy merge. Do not apply them.
- Record sample size, window, sound rate, and prompt-version mix in the report. No persistent audit ledger exists.
