---
name: build-spec
description: >
  Executes a promoted spec by spawning agents to work on tasks according to
  the plan's sequencing instructions. Reads a plan from
  agent-context/plans/ready/, moves it to in-progress, starts a feature
  branch, and coordinates task execution across phases. Use when a reviewed
  /draft-plan spec is ready for implementation.
disable-model-invocation: true
argument-hint: "[plan-name]"
---

# Build Spec

Execute a spec from `agent-context/plans/ready/`. Coordinate — don't produce. Dispatch agents, track progress.

Problem briefs (header line starts `Brief ·`) go to `/build-brief` instead.

## Available plans

!`ls agent-context/plans/ready/ 2>/dev/null || echo "(none)"`

## Process

### 1. Load the plan

Read `agent-context/plans/ready/$ARGUMENTS/index.md`. If missing, list available plans and ask which to run.

Understand:
- Goal section (every agent needs this)
- Each task's description, and the plan-level Acceptance criteria
- Boundary inventory, when present — the cross-task naming contract
- Sequencing: phases, concurrency, and dependencies
- **Surface** — `tools`, `web`, or both. Use the plan's `Surface:` line when it has one; otherwise infer from the paths its tasks name.

Then read `agent-context/lib/index.md` and route from the surface to the guides that govern it. Load only those:

| Surface | Guides |
|---|---|
| `tools` | `developer-guide.md`, `testing-guide.md` |
| `web` | `web-guide.md`, `web-testing-guide.md` — `web-guide.md` names the few `developer-guide.md` sections that still apply |
| Both | Both sets |

A coordinator carrying the wrong surface's conventions pastes them into dispatch packets, and the implementer follows them.

### 2. Move to in-progress and branch

Start on clean, current `main`. Move the plan, commit the move on `main`, then branch:

```bash
git mv agent-context/plans/ready/<plan-name> agent-context/plans/in-progress/<plan-name>
git commit -m "chore(plans): move <plan-name> to in-progress"
git switch -c feat/<plan-name>
```

`feat/<plan-name>` is the integration branch for all implementation work. Create agent worktrees from it and merge completed work back to it.

### 3. Execute phases in order

For each phase in the sequencing section:

**Sizing.** Set `model:` on every Agent call. Omitting it inherits the session model — Opus spent on plumbing, or contract work handed to Sonnet.

**Opus** when the task touches:
- Migrations, views, or anything other consumers read from the schema
- Write paths — snapshots, fetch runs, classifications, `mcp` action functions
- MCP tool shapes, envelopes, and role boundaries
- Cross-package data flow: adapter ↔ fetcher, SQL ↔ generated Go ↔ JSON, view ↔ web read
- Ambiguous acceptance criteria; design choices resolved by reading code

**Sonnet** for:
- Localized implementation with a clear package or component home
- Focused tests for already specified behavior
- Components and pages against a pinned view or type
- Mechanical propagation across call sites
- Small review fixes, low blast radius

**Split on contract risk, not task size.** Settled contract → Sonnet, whatever the size. The contract itself is the work → Opus. Sonnet lands localized features and tests near Opus quality; Opus on a bounded task costs more and widens scope past the acceptance criteria. Haiku is not an implementation agent here.

**One local contract is the Sonnet boundary.** Another package, a view consumer, or an agent-facing surface consuming the output means Opus — or split the task so the seam is its own Opus task.

**Briefing Opus on contract tasks.** Name what stays fixed: column names and nullability, migration numbers, JSON keys, envelope shape, role grants. Require a test that fails when a hand-mirrored shape drifts — a struct tag against a view column, a fixture against an envelope. Don't ask it to double-check its work; that buys over-verification, not coverage.

**Sequential:** One agent at a time on `feat/<plan-name>`. Wait for completion before starting the next.

**Concurrent:** Spawn all phase agents simultaneously via multiple Agent tool calls in one message, each in an isolated worktree.

> **Worktrees share one Postgres.** A worktree isolates files, never schema. Concurrent agents never run `migrate up`, pick a migration number, or run `sqlc generate` — the task that owns migrations runs sequentially, before its consumers. Concurrent agents gate on unit tests only; database-backed suites (`go test -tags=integration`, `pnpm test:db`) run on the branch after merge. A fresh worktree lacks `.env.local` links and `apps/web/node_modules`: link `.env.local` from the main checkout by absolute path, and run `pnpm install` in `apps/web` before any web command.

**For each agent, provide:**
1. The plan's **Goal** section
2. The agent's **specific task** description, plus the plan's Acceptance criteria section (AC is plan-level; the templates define no per-task AC)
3. The plan's **Boundary inventory**, when present — cross-task contract; break no row
4. The **surface** this task targets — `tools`, `web`, or both. It selects the guides in item 5 and the verify set the implementer runs. An implementer left to infer it from a file path will guess wrong on a task that touches one file outside its surface.
5. The relevant `agent-context/lib/` slices **inlined** — route via `agent-context/lib/index.md`, paste the sections that govern the task's subsystems. Agents under task pressure skip "go read X" instructions; paths drift.
6. The code-grounding rule: any claim about an identifier's shape or behavior comes from a file opened this session, not memory.
7. §3 Examine dependencies through §6 Report from `.claude/skills/implement-task/SKILL.md`, pasted verbatim — that skill owns the implementer process. Its §1–§2 (context and task loading) are superseded by items 1–5 of this packet. Concurrent agents in worktrees run its unit-level checks only; database-backed suites wait for merge.

This list is the dispatch contract. `/review-implementability` simulates it when reviewing specs; if the two drift, this list wins.

**Do NOT provide:**
- Other tasks' details (the agent doesn't need them)
- The full plan document (wastes context)
- Freedom to expand scope beyond acceptance criteria

### 4. Integrate results

After each phase:
- Read each agent's completion report: AC statuses, test output, deviations
- Verify acceptance criteria are met — trust the report's evidence, not its confidence
- A deviation that shifts contracts or scope is a surface-to-user event (developer-guide §1.2)
- If a task completed partially or blocked, surface to the user with context
- Merge worktree work back to `feat/<plan-name>`
- After a migration task merges, apply it to the development and test databases and run the role scripts `developer-guide.md` §2 names for that kind of migration. The coordinator applies migrations; agents never do.
- After merging a concurrent phase, run the touched packages' tests on the branch, database-backed suites included — this is where a concurrent phase's database verification lands

Between phases, check that prerequisites for the next phase are satisfied.

### 5. Complete

When all phases are done:
- Run the `/preflight` gates — the coordinator's single full gate. It detects the surfaces the change touched and runs each one's checks.
- Run a `/review-panel` on code edited in this session
- Report review panel findings to user to discuss which feedback to act on
- Run `/fix-findings` on the findings the user accepts
- Name the two or three choices this feature made that are worth the user's understanding — idiomatic Go on `tools`, interaction and design-system decisions on `web`. The project is a learning vehicle (project.md §Why it exists)

### 6. Landing the plane

When the user says "land the plane":
- Re-run `/preflight` when any code changed after the gate (fix-findings edits, hand fixes)
- Move the plan to done: `git mv agent-context/plans/in-progress/<plan-name> agent-context/plans/done/<plan-name>`
- If the plan is an item on `agent-context/plans/roadmap.md`, mark it done
- Remove session worktrees
- Commit & push `feat/<plan-name>`

### Error handling

- **Agent fails a task:** Surface the error and acceptance criteria to the user. Ask whether to retry, skip, or abort.
- **Merge conflict from concurrent agents:** Resolve if straightforward; escalate to user if the conflict involves architectural decisions.
- **Migration number collision:** Renumber the later migration before it is applied anywhere. Never renumber one that has run.
- **Preflight fails:** Fix if the issue is mechanical (formatting, a simple staticcheck lint, a moved import). Escalate if the fix requires design decisions.

### Principles

- **You coordinate, you don't produce.** Every tool call spent building is context not spent orchestrating.
- **Guard context.** Each agent gets minimum viable context for their task.
- **3 of 4 completing is enough.** Partial progress with clear status beats blocking on one stuck task.
- **Free tier only in agents.** Live fetch runs and batch-enrich waves are the coordinator's or the user's (developer-guide §2 Cost map).
- **Surface, don't guess.** Tell the user when something unexpected happens. Don't make architectural decisions on their behalf.
