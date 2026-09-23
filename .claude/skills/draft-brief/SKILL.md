---
name: draft-brief
description: >
  Drafts a problem brief — the lightweight spec form for a repo-aware
  executor. Records the problem, decisions, acceptance criteria, and a
  non-binding path. Chooses compact or resumable execution before prose
  grows around the work. Use after /draft-session routes work to a brief.
  Promotion follows /validate-plan and owner sign-off.
argument-hint: "[feature-name]"
---

# Draft Brief

Explore scope, choose execution weight, write a brief. Output lives in `agent-context/plans/drafts/<feature-name>/index.md`. The line under the title identifies the document and its execution mode.

A brief is written for a reader with the repo. One integrating executor owns the whole brief, `research.md`, and the source tree; it writes the task split and may delegate bounded slices with the same context. Write for that reader: nothing restated, nothing pre-chewed. The brief records judgment and leaves task decomposition to build time. Target 40–90 lines for compact work, 60–120 for resumable work. Longer material is derivation (`research.md`) or task decomposition (`plan.md`).

A problem brief is not a `/one-pager`. A one-pager is for `web` work the owner steers on screen, with no review gate. A brief carries decisions that must be reviewed before code exists, on either surface.

## Current plans

!`ls agent-context/plans/drafts/ agent-context/plans/ready/ agent-context/plans/in-progress/ 2>/dev/null`

## Rules

1. **Nothing is restated for an agent that cannot see the rest.** Every executor gets the whole brief, relevant research and context, and the source tree. There is no task-paragraph contract.
2. **Decisions in, verification out.** Ground the premise of every *Decision* against source this session — a decision built on a false premise is the expensive kind. Everything else is cited by symbol and left for the executor to re-verify: the header records the commit the source was read at, and a stale *Path* claim is reported in the plan of record, not fixed in a review round. No line numbers.
3. **One review gate, at direction.** `/validate-plan` runs once. No identifier-checking review, no implementability review. The diff is reviewed instead, by `/review-panel`. `/review-draft-spec` never runs on a brief: its lenses emit task-paragraph fixes the form has no home for, and applied they land in Decisions as binding clauses. Where the stakes warrant a detail read, `/review-brief` is the opt-in — it writes only Acceptance rows and `research.md`, and everything else is a finding for the owner.
4. **Execution weight follows coordination cost.** Cross-boundary contracts add detail to the brief. They do not alone require resumable execution. Choose resumable mode only when durable checkpoints or handoffs will earn their cost.

## Process

### 1. Frame and size

Start from the `/draft-session` handoff: problem, outcome, verified facts, decisions, proof, and the route. If none exists, run `/draft-session` first — routing is its job, not this skill's. Ask a focused question only where the handoff leaves the Problem paragraph unwritable.

Choose the execution mode:

| Mode | Use when |
|---|---|
| Compact | One coherent outcome can be built in one sustained session. This is the default. |
| Resumable | Work likely spans sessions, has ordered phases, needs durable handoffs, waits on later manual proof — a live fetch run, a batch-enrich wave — or has an irreversible migration. |

MCP tool changes, view changes, and `tools` ↔ `web` behavior still require complete Decisions, Acceptance, and boundary sections. They do not force resumable mode.

Revisit the mode after research. Change it before writing when new facts change the coordination cost.

### 2. Research

Read `agent-context/lib/style-guide.md` first. All brief prose follows it.

Route through `agent-context/lib/index.md` to the docs governing the surface. Grep `agent-context/plans/done/` for the *concepts* the brief touches — append-only, derived once in SQL, trust tiers, backend computes vs model narrates, read role vs action role — not just the package name. Cross-plan commitments are the ones a package-local drafter misses.

The handoff's verified facts are the floor, not the ceiling. Use subagents only for bounded, independent research questions — one claim, one agent, answered with the symbol. Stop when you can write Decisions with every premise grounded and the Problem's basis confirmed, not just reported.

Findings that inform but don't decide go to a sibling `research.md`. Data-flow diagrams go there too; keep one in the brief only when it is the clearest statement of a decision.

### 3. Write the brief

```markdown
# <feature-name>

Brief · <compact|resumable> · reads: `agent-context/lib/<doc>.md` §x · read at <short-sha>

## Problem
One paragraph. Who raised it — the owner using the tool, a data audit, a
review finding, an agent run — and its basis: an observed defect, a
requested capability, an anticipated need, or an experiment. A defect gets
its cause in one sentence, not a symptom of it; a capability gets the
behavior it adds and who uses it. Then what is true when this is done,
written as behavior.

## Decisions
- One bullet per decision: what, and why. Cite the commitment it touches
  (`agent-context/lib/…`, `plans/done/…`). Where it diverges from one, say
  so and argue it in a sentence. Note the undo cost only where it is not
  trivial — a migration always has one.
- Non-goals are decisions too. Add the warrant only where a reader would
  otherwise assume this brief owes the work ("requisition count on the
  status page: `requisition-identity` owns it; this brief reports postings").
- State the layer placement and the reason — derived-in-view vs
  per-consumer, write-time vs read-time, deterministic code vs agent
  judgment, read role vs action role, `tools` vs `web`; whichever axes are
  in play.

### Agent-facing surface
Only when the brief adds or changes an MCP tool, or a view the agent's
`query` tool reads. One example call with its response; normative for the
tool name, argument names and defaults, envelope shape, and error form;
silent on the SQL and Go behind it.

## Acceptance
Observable, edge-named, verifiable by someone who did not write the brief.
Orderings are rows here, not prose: a fetch that fails after a success, a
posting absent from one run then back, zero rows vs no run, a re-run over
rows already written. A guarantee that spans several rows gets a one-line
heading over them. Named types and functions do not appear here.

### Automated
- [ ] …
### Manual
- [ ] … (a route or story on screen, a live fetch run, an MCP call in a session, a query against real data)

## Path
Non-binding. Research distilled to what would change the executor's plan.
- Seams and precedents to investigate, by symbol. Exact reuse follows
  `agent-context/lib/style-guide.md` §Spec Completeness.
- The shape chosen and the strongest rival, one sentence each.
- The first slice: the thinnest path that falsifies the riskiest assumption.
- Files past ~600 lines this extends: split first, behavior-preserving,
  own commit (`developer-guide.md` §4).
- A code sketch only when it is the clearest statement of a decision.

## Open questions
- <question> — owner: <who> — **blocks build**
- <question> — **delegated**: the executor decides and reports it in the plan of record
```

**Decisions vs Path.** For each sentence: if the executor deviates from it, is that a defect or a note in the plan of record? Defect → Decisions. Note → Path. "The view resolves current snapshots within the latest successful run" is a Decision; which CTE carries the run predicate is Path.

**Agent-facing surface.** An MCP tool's shape, or the columns of a view the agent queries, is designed by the owner, and it is a Decision: once a skill or a prompt depends on it, it is a one-way door. The brief carries it as an example call under `### Agent-facing surface` inside Decisions, written the way an agent would call it. The example is normative for the surface and says nothing about the implementation behind it. It is also a fixture: one Acceptance row runs it, as a test or an MCP call in a session, so the example cannot drift from what ships. Path may sketch an alternative shape for the owner to weigh; Path never carries the one that ships.

**Argue in `research.md`; conclude in the brief.** A Decisions bullet that runs past three sentences is still arguing. State a fact once — a fact in two places is a defect waiting for a fix to land in one of them — and never write a count in prose; the enumeration stays right when the count goes stale.

**Size smell** is on the Problem paragraph, not the document. Two problems in one paragraph is two briefs. Past the mode's target, move derivation to `research.md` and task decomposition to the executor. A required boundary table does not count toward the target.

**Cross-boundary names.** When the brief crosses Go ↔ JSON ↔ SQL, append the `Boundary inventory` section from `/draft-plan` unchanged. There the document *is* the contract between sides built separately, and the brief is only its front half.

### 4. Cross-check

- Every Acceptance row: which Decision or Problem sentence makes it necessary? None → it is aspirational; drop it or add the decision.
- Every Acceptance row: could it pass on a build that leaves the Problem unsolved — the defect present, the capability absent? Yes → it is measuring something adjacent; reword it, or label it a regression guard.
- Every Decision: which Acceptance row would fail if it were violated? None → it is either a Path hint wearing a decision's clothes, or an AC is missing.
- Every Decision premise about the code or schema: read this session, cited by symbol.
- Every exact-reuse Path claim meets `agent-context/lib/style-guide.md` §Spec Completeness.
- Every write path: proof covers what a re-run, a partial failure, and a concurrent writer leave behind, not only the happy insert.
- Every "not doing": would a reader assume this brief owed it? If so, it carries a warrant.
- The Agent-facing surface example, if present: an Acceptance row runs it, and every name in it either resolves against the MCP server or is one this brief adds.
- Open questions: each is marked **blocks build** or **delegated**. No unmarked entries.

### 4b. Revising

- A resolved question becomes one Decisions bullet and leaves. No history: not "was open," not "we settled on." The executor never saw the question.
- A review finding lands as an Acceptance row, a `research.md` pin row, or a Decisions edit the owner makes in their own words. Reviewer prose never lands in Decisions — that parenthetical is how a brief turns back into a spec.
- Re-read Decisions after any Problem edit. A reframed basis orphans a decision that answered the old one, and the diff never touches the orphan.
- Read the diff, not the result. Amend-only histories destroy the per-round diff, so snapshot before each round and diff against the snapshot. An edit that drops a trailing line reads fine everywhere you think to look.

### 5. Commit

For resumable drafting, stage and commit the plan folder. Amend as the brief iterates in-session; one commit per brief, not one per edit.

For compact work continuing in the same session, keep the draft uncommitted until promotion. Commit sooner when the session may end or the owner wants a durable review point.

Do not update `agent-context/lib/` during drafting. Durable capture happens at promotion.

### 6. Validate direction

Run `/validate-plan <name>`. It reads the brief the same way it reads a spec; the six questions apply unchanged.

Surface the verdict and read it as a fresh reader would; do not rebut it from inside the session that drafted the brief. Never act on *Reshape*, *Not a spec*, or *Under-scoped* unilaterally — those are owner decisions.

### 7. Report

- The problem, in one line
- Decision count, AC count, open-question count by kind
- The `/validate-plan` verdict
- Open questions marked **blocks build**, for the owner
- The brief lives in `drafts/` until promoted

## Working open questions

Between draft and promotion the owner and the drafter resolve **blocks build** questions. A resolution becomes one Decisions bullet, and the Open questions entry is removed. Re-run `/validate-plan` only when a resolution changes the Problem paragraph or swaps the chosen shape for a rival; a resolution that pins a value or a mechanism does not need it.

## Promoting to `ready/`

A brief is ready when:
- `/validate-plan` returned *Direction sound*, or the owner accepted a reshape and the brief reflects it
- No **blocks build** entries remain; every surviving entry is **delegated**
- The owner signs off

At promotion:
1. Capture durable decisions in `agent-context/lib/` — new constraints, package contracts, pipeline topology.
2. `git mv agent-context/plans/drafts/<name> agent-context/plans/ready/<name>`
3. Commit the move and the `agent-context/lib/` updates together.

## What happens after

`/build-brief` reads the mode from the header. Both modes write `plan.md` with corrections, delegated answers, task split, and an AC-to-proof table.

- **Compact:** write the plan, commit it with the move to `in-progress/`, and continue. Promotion plus invocation is approval; there is no second plan stop.
- **Resumable:** commit the proposed plan and stop for the owner's skim before implementation.

**Decisions and Acceptance are owner-owned.** A material change to either requires a proposed restatement and an owner decision. A clarification that preserves both goes in `plan.md`; it does not stop the build. A false Decision premise always stops.

At landing the table gains a result column — every AC, its proof, pass or fail; a gap is named, never silent — and the brief moves to `done/` with `plan.md` beside it.

**Opt-in pre-build check.** When the stakes warrant it — a migration, a write path, an MCP tool shape, a view other consumers read — run `/review-brief` before promotion. It fact-checks Decision premises, pins orderings as rows, asks whether each AC is achievable and proves the Problem, and asks what could be deleted; it edits only Acceptance and `research.md`, and every finding on Decisions is the owner's. Not a default step, and never a second round — a brief that needs one has a Decisions problem, which is `/validate-plan`'s.
