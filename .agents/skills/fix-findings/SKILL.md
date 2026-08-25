---
name: fix-findings
description: Fix confirmed review findings by delegating independent edits first and cross-cutting fixes second. Use after the user accepts findings from review-panel or another review.
---

# Fix Findings

Coordinate fixes. Do not start from unverified reviewer claims.

## Verify

Open current source for every finding.

- Confirmed: triage it.
- Refuted or already handled: drop it and record why.
- Uncertain: surface it. Do not dispatch a fix.

Default scope: accepted red and yellow findings. Fix a green only when it is trivial and in a file already being changed.

## Triage

| Class | Dispatch |
|---|---|
| One file, no contract change, no likely knock-on effects | Group by file. Delegate independently and concurrently. |
| Cross-file or package change, interface or contract change, architectural judgment | Delegate one at a time with likely knock-on targets. |

Every implementation brief includes the finding, evidence, expected fix direction, relevant context files, and the surface-specific verification commands. Require source grounding for identifier claims.

## Integrate

Read each completion report. Check its evidence, then run the appropriate final verification. Surface partial work and remaining choices.

## Report

State what was fixed, refuted, uncertain, skipped, and verified.
