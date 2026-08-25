---
name: review-panel
description: Run a source-grounded, multi-agent review panel with specialist lenses, a comment-drift pass, deduplication, and refutation. Use after implementation or before a pull request.
---

# Review Panel

Review the diff through separate lenses. Do not review it yourself before delegation.

## Scope

Review a named plan, path, or uncommitted changes. Partition diffs above roughly 1,500 lines by package into slices of about 1,000–1,500 lines. Keep smaller diffs in one slice.

## Process

1. Delegate a fast triage per slice. Identify flows, contract surfaces, subtle invariants, write paths, and exit flows.
2. Run these independent lenses for every non-mechanical slice:
   - Correctness tracer: execute one flow end-to-end across its producers and consumers.
   - Contract verifier: compare migration, SQL, generated code, types, JSON, docs, and runtime behavior.
   - Adversarial tester: construct grounded empty, NULL, cancellation, collision, and repeat-call cases.
   - Data-integrity reviewer: run when the slice writes rows; check append-only, atomicity, provenance, and NULL semantics.
   - Hygiene and drift reviewer: always run. Check basic code quality and comments in changed and adjacent code.
3. Run up to three cross-slice seam traces when flows exit a slice.
4. Dedupe findings. Keep the precise description, count agreement, and retain the higher severity.
5. Refute every red and yellow code finding with an independent source pass. Keep a finding only when the refuter cannot disprove it. Comment-drift findings require a quote from current on-disk text instead.

## Hygiene and drift

Read `developer-guide.md` comment guidance and `style-guide.md`. Check stale file headers, behavior comments that no longer match code, missing why-comments for non-obvious decisions, orphan TODOs, comments that restate code, broken spec pointers, and adjacent comments naming contracts changed by the diff.

## Report

Use `red`, `yellow`, and `green` severities. Keep comment drift separate from code findings. For every surviving finding, give `file:line`, lens, agreement count, problem, evidence, and proposed fix. List refuted findings separately with the citation. Any red requests changes; yellow-only needs discussion; otherwise approve.
