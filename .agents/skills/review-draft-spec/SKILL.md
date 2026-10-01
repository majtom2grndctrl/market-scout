---
name: review-draft-spec
description: Review a draft plan with parallel broad and source-anchored lenses, apply mechanical corrections, and recommend the next promotion step. Use after drafting a plan in `agent-context/plans/drafts/`.
---

# Review Draft Spec

## Model choice

Read [Codex Model Guide](../../../agent-context/lib/model-guide.md) for identifiers, availability, and effort controls. Use Sol at `high` for both independent reviewers; use Astra at `high` for unresolved architectural questions.

Run two reviewers in parallel. One checks the spec itself; one verifies every named identifier against source.

## Process

1. Resolve the requested draft and read it in full.
2. Delegate two read-only reviews. Inline the spec; paths drift.
3. Give the broad reviewer relevant context slices. It checks contradictions, scope, Task-to-AC coverage in both directions, boundary casing, plumbing, missing wire-format pins, and dense task prose that stacks constraints or scatters prohibitions.
4. Give the source reviewer the spec and require it to extract every named identifier, open current source, and cite each divergence.
5. Deduplicate findings. Keep the source-anchored framing when both report the same issue.
6. Classify findings as Blocker, Complicates, or Nit. Apply mechanical corrections unless `--no-auto-apply` is set. Surface architectural choices.
7. Re-read the draft after changes.

## Recommendation

| Outcome | Next step |
|---|---|
| Only resolved nits | Run `review-implementability`, then promote. |
| Mechanical fixes, no architectural findings | Re-run this skill once. |
| Architectural findings | Surface choices. Do not promote. |
| Only source-reading findings | Run `review-implementability`, then promote. |

## Report

Name reviewers, findings by severity, applied mechanical change count, unresolved architectural findings, and recommendation. Keep it concise.
