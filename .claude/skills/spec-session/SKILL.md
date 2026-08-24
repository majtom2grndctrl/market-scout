---
name: spec-session
description: >
  Grounds an agent at the start of a spec-writing or spec-revising session
  with habits that keep defects out of a draft. Invoke first, before any
  drafting or review skill.
---

# Spec Session

## Before you write

Read these skills and files before drafting:

- **`agent-context/lib/style-guide.md`** - Contains writing principles to follow for specs and code comments, like "direct and brief" and "seamless". Follow these guidelines for any prose written for the benefit of other agents.
- **`/draft-plan`** — spec format, task-paragraph contract, sequencing rules, cross-check discipline. A spec is not a wish list; it is a grouping of tasks executed in coordination to produce a coherent unit of work. `/orchestrate` dispatches those tasks — phased, sized, briefed — so the spec must be machine-readable at that grain.
- **`/orchestrate`** — how task agents receive context (Goal + their paragraph + AC list + Invariants table, nothing else), how phases sequence, how concurrent agents isolate. Write the spec knowing this is the consumer.

**Build more right faster.** AI coding agents produce code quickly enough that incremental baby-steps waste more time than they save. When table stakes are well known — or the destination is already clear — spec the full shape and build it, rather than reinventing known ground one slice at a time. Small increments earn their cost only when the path is genuinely uncertain. A spec session's job is to resolve that uncertainty up front so implementation can move in confident strides. When a spec lays a foundation, build its first consumer in the same unit of work — the consumer proves the foundation and keeps it from shipping as a stub nothing exercises.
