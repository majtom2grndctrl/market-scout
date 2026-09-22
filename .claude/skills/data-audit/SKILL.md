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

Pull N recent classifications joined to their postings, via read-only SQL against the Docker Postgres. Current classification per posting is the latest row by `classified_at` — the `DISTINCT ON` shape in `apps/tools/internal/db/queries/classifications.sql` is canonical; read it before writing ad-hoc SQL. Include per item: title, description, assigned roles, specializations, skills, seniority, `notes`, summary, `prompt_version`.

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

**Judging seniority.** The classifier contract abstains by design: `unknown` is a correct, often-majority verdict, not evidence of `wrong-role`. `mid` and `junior` come only from an explicit level word in the title or body; absent that, the contract can only resolve to `senior` or `director`, and only against a verbatim quote. Judge a seniority call against that rule, not against whether a rung "feels" assignable from the description as a whole: `unknown` needs no supporting phrase; any other rung needs one quoted verbatim from the posting. A rung asserted without a quotable phrase is `wrong-role`; `unknown` on a posting that plainly names no level word and quotes no scope signal is not.

The quote lives in `notes` as `seniority[<tag>]: "<phrase>"`, where the tag names the step that produced the rung. Check that the tag matches both where the quote came from and what class it falls in:

| Tag | Holds when |
|---|---|
| `step1-title` | The quote is in the title and carries a level word |
| `step1-body` | The quote is in the description body and carries a level word |
| `step2-org` | The rung is `director`, and the quote heads a function or org, owns a P&L, or manages managers — per batch-enrich's class (a) list. A reporting line, to the C-suite or anyone else, never holds |
| `step2-manages` | The rung is `senior`, and the quote shows this role managing individual contributors |
| `step2-align` | The rung is `senior`, and the quote uses one of batch-enrich's class (c) qualifying verbs, not a collaboration verb such as "partner with" |

A tag that does not match its quote, a `step2-*` tag on any rung other than `senior` or `director`, or a missing tag on a non-`unknown` rung is `wrong-role`. The tag is how analyses separate stated seniority from inferred, so a wrong tag corrupts that split as surely as a wrong rung. `unknown` carries no seniority note. Rows from versions before the tag existed carry the untagged `seniority: "<phrase>"` form; judge their quote, and do not flag the missing tag.

### 3. Drift checks (coordinator — no agents)

- `agent-output/batch-enrich/failures.jsonl`: failure count and dominant mode per recent run. A rising rate or a new mode is a flag.
- Taxonomy growth: emergent entries in the window; near-duplicate slugs across `canonical_roles` and `specializations`.
- `prompt_version` mix: when the window spans a contract change, split verdict quality by version. `prompt_version` is the primary audit key (§6.2). Each writer pins its own value — never infer a version from a model, and never assume one version means one contract. Four pre-2026-09-21 cohorts are ambiguous or unattributed; §6.2 names them. Report them as unresolved rather than folding them into a neighbouring version. Never compare seniority distributions across prompt versions: a version bump can reshape the seniority contract on purpose — v9 made `unknown` common and confined `mid`/`junior` to an explicit level word — so a shift in the `unknown` share, or any rung's share, between versions is the contract working as designed, not drift. Compare seniority only within one `prompt_version`.

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
