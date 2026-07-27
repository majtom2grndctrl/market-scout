---
name: data-audit
description: >
  Read-only audit of agent-written data quality. Samples recent
  classifications, judges them against source descriptions via parallel
  agents, and checks failure and taxonomy drift across runs. Use
  periodically during active collection, or after changing the classifier
  contract, boilerplate stripping, or a discovery flow.
allowed-tools: Read, Glob, Grep, Bash, Agent
argument-hint: "[sample:N] [since:YYYY-MM-DD]"
---

# Data Audit

Code review catches code bugs. This catches data bugs — the kind that corrupt trend history silently and surface months later in queries, the most expensive place to find them. Read-only: the audit never writes to the DB.

Read `agent-context/lib/developer-guide.md` §6.2 first. It names the inspection surfaces and invariants this audit checks.

## Defaults

- Sample: 20 classifications (`sample:N` to override)
- Window: last 14 days (`since:` to override)
- Judges: Sonnet, one agent per ~5 sampled items

## Process

### 1. Sample

Pull N recent classifications joined to their postings, via read-only SQL against the Docker Postgres. Current classification per posting is the latest row by `classified_at` — the `DISTINCT ON` shape in `apps/tools/internal/db/queries/classifications.sql` is canonical; read it before writing ad-hoc SQL. Include per item: description, assigned roles, specializations, skills, summary, `prompt_version`.

Stratify the sample: mix companies and ATS sources. Include emergent taxonomy entries when the window has any (`created_at` distinguishes emergent from seeded — §6.2).

### 2. Judge (parallel Sonnet agents)

Each judge receives its items inline — description plus assigned classification. Judges never see each other's items. One verdict per item:

| Verdict | Meaning |
|---|---|
| sound | Classification matches the description |
| wrong-role | Role or specialization does not fit the description |
| junk-summary | Summary is generic, boilerplate, or contradicts the description |
| suspect-taxonomy | Emergent entry duplicates or fragments an existing one |

Each verdict carries one sentence of evidence quoting the description. No praise, no padding.

### 3. Drift checks (coordinator — no agents)

- `agent-output/batch-enrich/failures.jsonl`: failure count and dominant mode per recent run. A rising rate or a new mode is a flag.
- Taxonomy growth: emergent entries in the window; near-duplicate slugs across `canonical_roles` and `specializations`.
- `prompt_version` mix: when the window spans a contract change, split verdict quality by version. `prompt_version` is the primary audit key (§6.2).

### 4. Report

- Sound rate with its denominator — never a rate without the sample size
- Findings grouped by pattern, not by item, each with its evidence
- Drift flags
- Recommended follow-ups — `--force` re-classification of a subset, a boilerplate-stripping fix, a taxonomy merge. Suggestions only; nothing is applied.

No persistent audit ledger exists. Record the headline numbers (sound rate, sample size, window, `prompt_version` mix) in the report so the next audit has a baseline to quote. A ledger is a future decision — do not invent one.

## Working rules

- Read-only. The audit queries; it never writes, merges, or re-classifies.
- Judges receive content inline. Paths drift.
- Confirm any claim about an identifier's shape by opening the file this session — never from memory.
