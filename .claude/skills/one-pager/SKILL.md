---
name: one-pager
description: >
  Writes a one-page build brief for frontend work in apps/web — goal, target
  call-sites, decisions, non-goals, done-when. Use for hands-on Next.js, React,
  Tailwind, Storybook, or UI work where the user steers implementation directly
  and verifies by looking at the result. Not for work dispatched to parallel
  agents — use /draft-plan for that.
disable-model-invocation: false
allowed-tools: Read, Glob, Grep, Bash, Write, Edit
argument-hint: "[feature description]"
---

# One Pager

Write a build brief for frontend work. Output: `agent-context/plans/in-progress/<feature-name>/index.md`.

## When this instead of /draft-plan

The heavy pipeline exists because backend verification is expensive — DB state, cron, live ATS APIs. Reading a spec catches mistakes more cheaply than running the thing. Frontend inverts that: the running thing is the verification artifact, and the user is the domain expert reading it.

Use `/one-pager` when the user is in the loop and steering. Use `/draft-plan` when work will be fanned out to parallel agents through `/orchestrate` — a brief underspecifies for agents nobody is watching, and they diverge on exactly the details it omits.

Say which one this is if the request is ambiguous. Don't guess silently.

## What earns a place

Filter by **expensive to reverse and invisible on screen**. Directory layout, generated-vs-handwritten boundaries, API shape, dependency choices, and the reason a tempting approach is wrong — none of those show up in a browser, and all cost real work to undo.

Anything the user verifies by looking does not go in the brief. It goes on the screen.

## Process

### 1. Understand the goal

Ask focused questions only where two readings produce different work. The user has the design context; don't re-derive it out loud.

### 2. Ground it

Read the files the work touches. Check current library behavior through context7 for Tailwind, Next.js, or Storybook APIs — version drift is the largest source of wrong briefs, and memory does not track minor versions.

Code-grounding rule: every identifier the brief names — component, prop, token, path, script — comes from a file opened this session or a doc fetched this session. Not from memory.

Findings that don't drive a decision stay out. If they're worth keeping, put them in a sibling `research.md`.

### 3. Write it

Follow `agent-context/lib/style-guide.md` — **Direct and brief**, **Seamless**. §Density vs. Clarity governs length: no wasted words, not fewest words.

```markdown
# <Feature Name>

## Goal
What this achieves and why it matters now.

## Target usage
Code sketch. What call-sites look like when this is done.

## Decisions
- <Decision> — <reason>.

## Not doing
- <Thing> — <why it's tempting, why it's wrong here>.

## Build order
(Optional. Numbered, one line per step. For a human steering — not phases for /orchestrate.)

## Done when
- Commands that pass.
- What the user looks at.

## Open questions
(Omit when empty.)
```

### 4. Confirm

Show the brief and take corrections before moving on. The user reads faster than they write — this is the cheapest review in the workflow, and it replaces the three reviewer passes `/draft-plan` needs.

### 5. Commit

Stage and commit the plan folder. Capture durable decisions in `agent-context/lib/` in the same commit — a brief short enough to reread can serve as its own record, but architectural constraints that outlive the feature still belong in the library.

## Section budgets

Length is governed per section, not per document. A brief that runs long because one section grew is a signal about that section — not a reason to compress the others.

| Section | Budget | Overflow means |
|---|---|---|
| Goal | 2–3 sentences | Scope covers more than one feature. Split it. |
| Target usage | 2–4 call-sites | Showing the API's surface, not its variations. Cut to the shapes that differ. |
| Decisions | One line each, reason attached | A decision needing a paragraph is a research finding. Move it to `research.md` and link. |
| Not doing | One line each | Fine to run long. This section is the highest value per line — it's the knowledge an agent gets wrong by default. |
| Build order | One line per step | More than ~8 steps means this wants `/draft-plan` and real sequencing. |
| Done when | Commands, plus one visual check | Enumerating what the user would see anyway. Cut to what a command proves. |

Rationale fuses to its constraint. A separate rationale paragraph is the part that gets skipped — then the constraint it protected gets simplified away (style-guide §Task Instructions).

## What does not go in

Omitting these is the point of the format. Each one is `/draft-plan` machinery that pays for itself only when nobody is watching the implementer.

Do not include:
- Acceptance criteria that a glance at the screen verifies. That's what the screen is for.
- A review-gates section. If it reads "a human checks this in Storybook," delete it — the human is checking it in Storybook.
- Sequencing phases with concurrency annotations. Build order is a list for a person.
- Exhaustive prop, token, or class enumerations. Those are codegen output or the type definition. Name where they live.
- A Task → AC cross-check. There are no tasks and little AC.
- A boundary inventory. Go ↔ JSON ↔ SQL casing is a backend concern.

## Lifecycle

Two states: `in-progress/` → `done/`. No `drafts/` or `ready/` — those stages exist to hold a spec between review gates, and this format has no review gates.

Move the folder to `done/` when the work ships. Don't maintain it after — the implementation is the source of truth (style-guide §Documentation Lifecycle).
