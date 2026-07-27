---
name: draft-plan
description: >
  Drafts feature or epic specs. A session may produce zero, one,
  or several plans depending on scope. Use when starting new planning work.
  Does not promote to ready/ — that is a separate step after review.
---

# Draft Plan

Explore scope, write specs. Output lives in `agent-context/plans/drafts/<feature-name>/index.md`.

A drafting session may produce 0, 1, or N plans. Scope often shifts during planning — let it. Don't lock a feature name before scope settles.

## Current plans

!`ls agent-context/plans/drafts/ agent-context/plans/ready/ agent-context/plans/in-progress/ 2>/dev/null`

## Process

### 1. Understand the goal

Read the user's description. Ask focused questions when scope is unclear — don't over-interrogate.

Pin down:
- What outcome matters
- What constraints apply
- What subsystems are touched
- What "done" looks like — concrete, verifiable

### 2. Research

Read `agent-context/lib/style-guide.md` first. All spec prose follows it.

Load relevant library files:

!`ls agent-context/lib/`

Use subagents for exploration — codebase reading, pattern discovery, doc lookup. Target 80% confidence. Stop when you have enough to spec the work.

**Code-grounding is non-negotiable.** Every Go identifier the spec will name — function, struct, type, field, interface — must be confirmed against current source before the spec asserts anything about it. Don't write "X returns Y" or "X has fields A, B" from memory. Open the file, read the signature, then write. Memory drift is the largest single source of spec inaccuracy.

**Research notes stay out of the spec.** If findings are useful but don't drive decisions, put them in a sibling `research.md` in the plan folder. The spec captures decisions and behavior, not the investigation that produced them.

### 3. Write the spec

Create `agent-context/plans/drafts/<feature-name>/index.md`.

```markdown
# <Feature Name>

## Goal
1–3 sentences. What this achieves. Why it matters.

## Scope

### In scope
- Bullet list.

### Out of scope
- Explicit non-goals. No "TBD" — decide or drop.

## Acceptance criteria
- [ ] Verifiable conditions for "done."

## Tasks
(Optional for small plans. Use when work splits cleanly.)

### Task 1: <name>
One paragraph. What to build.

### Task 2: <name>
...

## Sequencing
(Required when Tasks section exists. Feeds /orchestrate.)

**Phase 1 (sequential):** Task 1 — blocks everything.
**Phase 2 (concurrent):** Task 2, Task 3 — independent.
**Phase 3 (sequential):** Task 4 — consumes Task 2/3 output.

## Rough sketch
(Optional.) Implementation direction, key modules, algorithm hints. Named types and functions live here, not in AC.

## Boundary inventory
(Required when the plan crosses Go ↔ JSON/HTTP ↔ SQL boundaries. Skip otherwise.)

Pin casing and encoding once for every cross-boundary name. Reference this inventory throughout the spec instead of re-deciding inline.

| Name | Go struct field | JSON key | SQL column |
|---|---|---|---|
| (example) `CompanyID` | `CompanyID` | `"company_id"` | `company_id` |

## Open questions
Unresolved items, risks, alternatives considered.
```

**Length smell:** most plans land at 50–200 lines. Past 250 lines usually means the spec carries research notes (→ `research.md`) or scope should split into multiple plans.

**Plumbing rule.** Every "edit X to do Y" instruction must say how X gets access to what it needs. New side-tables need owners. New struct fields need writer call-sites. Function signature changes need their callers enumerated. Don't punt access plumbing to the implementer — the implementer has less context than the spec author.

**Write tasks for the implementer's tier.** Task prose follows `agent-context/lib/style-guide.md` §Task Instructions: one constraint per bullet with its why attached, prohibitions collected in one Do-not list, precedents as a Mirror/Don't-mirror table. The implementing model lossy-compresses dense paragraphs; it never flags them.

### 4. Acceptance criteria

AC names observable behavior. Someone who didn't write the plan must be able to verify it without reading the implementation.

| Too loose | Right | Too strict |
|---|---|---|
| "Syncing works" | "`GET /jobs` returns all active listings from configured ATS sources; stale listings absent" | "`JobsHandler.List()` queries with `WHERE status = 'active'`" |
| "Errors are handled" | "ATS fetch failure logs a structured error and skips that source; other sources still sync" | "`lever.Fetch()` returns wrapped error containing HTTP status code" |
| "It's fast enough" | "Full ATS sync completes in < 30s for up to 1000 listings on a standard run" | "HTTP client timeout set to exactly 5s in `newHTTPClient()`" |

Named types, functions, and line numbers belong in the sketch — not AC. AC survives a rewrite of the implementation; a spec keyed to function names does not.

### 5. Sequencing

Feeds `/orchestrate`. Terse is fine — models read short phase blocks reliably.

Rules:
- Concurrent by default.
- Sequential only when a later task consumes an earlier one's output, shares files, or breaks a contract if parallelized.
- Name the dependency in one clause ("Task 3 consumes the vertex format from Task 2"). No essays.
- Each phase completes fully before the next begins.

One phase per line. No per-task sub-bullets unless a dependency needs calling out.

### 6. Cross-check

Before committing, walk the spec twice:

- **Task → AC.** For every task line item, ask: "What AC verifies this behavior?" If nothing does, either the AC is missing or the task should drop.
- **AC → task.** For every AC, ask: "Which task produces the behavior this verifies?" If nothing does, either the task is missing or the AC is aspirational.

Both directions must close. Gaps signal that something was assumed without being written down.

### 7. Commit

Stage and commit the plan folder (`index.md` + optional `research.md`).

**Do not update `agent-context/lib/` during drafting.** Durable capture happens at promotion — after review. Reviewer agents often reshape the spec; library updates should land once, against the final shape.

### 8. Report

- What was planned, or if the session produced no plan (scope already covered, etc.)
- Task count and phase summary
- Open questions left for the user
- Plan lives in `drafts/` — not ready for `/orchestrate` until promoted

## Promoting a plan to `ready/`

Not part of the drafting session. Happens after review — often after reviewer agents pass.

A draft is ready when:
- Scope in/out is decided — no "TBD" markers
- AC is verifiable by someone who didn't write the plan
- Open questions are resolved, or explicitly scoped as decisions-during-implementation
- User signs off (reviewer agents may run first)
- A reviewer agent (or panel) can only find issues by reading source code, not by reading the spec. Issues that surface only at code-anchor depth signal the spec has hit diminishing returns — promotion is appropriate.

At promotion:
1. Capture durable decisions in `agent-context/lib/` — new architectural constraints, subsystem contracts, pipeline topology. Agents working the plan find full context in the library, not in the plan document.
2. `git mv agent-context/plans/drafts/<name> agent-context/plans/ready/<name>`
3. Commit the move and the `agent-context/lib/` updates together.
4. Run `/review-implementability` on the promoted spec before `/orchestrate`.
