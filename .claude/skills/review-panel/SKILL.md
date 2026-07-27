---
name: review-panel
description: >
  Runs a multi-agent review panel of specialized review lenses, sized to the
  diff by a triage step and refutation-checked before reporting. Executes as
  a deterministic workflow so phases and verification never depend on
  coordinator attention. Use mid-session after implementing a feature, or
  before opening a pull request.
allowed-tools: Read, Glob, Grep, Bash, Workflow, Agent
argument-hint: "[file-path | plan-name] [reviewers:N] [model:opus|sonnet]"
---

# Review Panel

Coordinator for a lens-based review panel. Partition the diff into slices, hand them to the workflow, present the results. Do not review code yourself.

## Panel model

A lens is a way of reviewing, not a region of code. The workflow (`.claude/workflows/review-panel.js`) triages each slice and dispatches from what the diff actually contains:

- **Correctness tracer** (depth) — mentally executes one data flow end to end. Ordering bugs, producer/consumer mismatches, swallowed errors, broken per-source isolation.
- **Contract verifier** (depth) — checks one changed surface for agreement across every layer: migration, SQL, generated Go, struct tags, JSON envelope, doc, runtime.
- **Adversarial tester** (depth) — concrete edge-case sequences: empty batches, NULL translation, cancellation mid-run.
- **Data-integrity reviewer** (depth) — dispatched when a slice writes rows. Append-only violations, atomicity, provenance. A code bug costs a rerun; a write-path bug corrupts history that cannot be refetched.
- **Hygiene + drift** (breadth, always Sonnet) — checklist pass plus comment integrity.

One agent per depth lens; one agent for the whole breadth cluster. Mechanical slices get the breadth pass alone — the panel scales down on small diffs.

Findings are deduped and severity-merged. Red and yellow code findings survive only when a Refuter fails to disprove them against live source with a citation. Comment-drift findings are not refuted — the hygiene reviewer verifies them by quoting live on-disk text instead.

## Scope detection

Determine review target from first argument (same rules as `/code-review`):

- **Plan name:** all files touched by the plan's tasks
- **File path:** that file and closely related files
- **No argument:** uncommitted changes

!`git diff --stat HEAD 2>/dev/null`
!`git diff --stat --cached 2>/dev/null`
!`ls agent-context/plans/in-progress/ agent-context/plans/done/ 2>/dev/null`

## Process

### 1. Parse arguments

Extract from `$ARGUMENTS`:
- The review target (plan name, file path, or empty for uncommitted changes)
- `reviewers:N` — force exactly N depth agents per slice, bypassing the dispatch table
- `model:opus|sonnet` — model for triage, depth, and refuter agents (default: opus)

### 2. Partition slices

Slices are `{ name, paths: [string] }` — the workflow rejects any other shape.

`git diff --stat` gives the line and file counts. Under ~1,500 changed lines: one slice, named for the change, `paths` = every touched path from the stat.

Above that, partition by package path (`cmd/mcp`, `internal/ats`, `internal/db`, …) into ~1–1.5k-line slices, working from the stat alone — don't read the diff to split it. One triage pass can't hold a diff that large: it names the obvious flows and misses the rest. Per-slice keeps triage sharp and each agent's load small.

### 3. Run the workflow

```
Workflow:
  scriptPath: .claude/workflows/review-panel.js
  args: { target, diffRef, slices, reviewers, depthModel }
```

`diffRef` is `HEAD` for uncommitted changes, or the ref range that covers the target. The `model:` argument maps to `depthModel`; it and `reviewers` pass through only when the user set them.

If the Workflow tool is unavailable, run the same stages with Agent calls — triage, lenses, seam, dedupe, refute. Replicate the script's dispatch rules (`buildLenses` caps, refute batching), not just its prompts.

### 4. Present unified review

```
## Review Panel Summary

**Panel:** [per slice: lenses that ran — depth lenses on the model arg, hygiene and dedupe on sonnet]
**Scope:** [single slice, or "N slices + seam pass"]
**Target:** [what was reviewed]
**Refuted:** [count dropped in refutation, one line each]
**Verdict:** approve / request changes / needs discussion

## Code Review Findings

### 🔴 Must fix
[Tag each with lens and agreement, e.g. "(tracer, 2x)"]

### 🟡 Should fix
[...]

### 🟢 Nits
[...]

## Comment Drift Findings

[Same severity structure — keep separate from code findings]

## What's done well
[The workflow's praise array, deduplicated]
```

Severity mapping: red → 🔴, yellow → 🟡, green → 🟢. Verdict rule: any surviving red → request changes; only yellows → needs discussion; otherwise approve. Omit empty categories. If the panel approves with no findings, say so clearly.
