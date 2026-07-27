---
name: orchestrate
description: >
  Orchestrates execution of a plan by spawning agents to work on tasks according
  to the plan's sequencing instructions. Reads a plan from agent-context/plans/ready/,
  moves it to in-progress, and coordinates task execution across phases.
  Use when a reviewed plan is ready for implementation.
disable-model-invocation: true
argument-hint: "[plan-name]"
---

# Orchestrate

Orchestrate a plan from `agent-context/plans/ready/`. Coordinate — don't produce. Dispatch agents, track progress.

## Available plans

!`ls agent-context/plans/ready/ 2>/dev/null || echo "(none)"`

## Process

### 1. Load the plan

Read these context library files first:
- `agent-context/lib/index.md` — agent router, architectural principles
- `agent-context/lib/developer-guide.md` — conventions, constraints, coding standards
- `agent-context/lib/testing-guide.md` — what to test, test patterns

Then read `agent-context/plans/ready/$ARGUMENTS/index.md`. If missing, list available plans and ask which to run.

Understand:
- Goal section (every agent needs this)
- Each task's description and acceptance criteria
- Sequencing: phases, concurrency, and dependencies

### 2. Move to in-progress

```bash
git mv agent-context/plans/ready/<plan-name> agent-context/plans/in-progress/<plan-name>
```

Commit the move.

### 3. Execute phases in order

For each phase in the sequencing section:

**Sequential:** One Opus agent at a time. Wait for completion before starting the next.

**Concurrent:** Spawn all phase Opus agents simultaneously via multiple Agent tool calls in one message.
**For each agent, provide:**
1. The plan's **Goal** section
2. The agent's **specific task** description, plus the plan's Acceptance criteria section (AC is plan-level; the templates define no per-task AC)
3. The relevant `agent-context/lib/` slices **inlined** — route via `agent-context/lib/index.md`, paste the sections that govern the task's subsystems. Agents under task pressure skip "go read X" instructions; paths drift.
4. The code-grounding rule: any claim about an identifier's shape or behavior comes from a file opened this session, not memory.
5. §3 Examine dependencies through §6 Report from `.claude/skills/implement-task/SKILL.md`, pasted verbatim — that skill owns the implementer process. Its §1–§2 (context and task loading) are superseded by items 1–3 of this packet.

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
- If using worktrees, merge completed work back to the main branch

Between phases, check that prerequisites for the next phase are satisfied.

### 5. Complete

When all phases are done:
- Run `/preflight` — the coordinator's single full gate
- Run a `/review-panel` on code edited in this session
- Report review panel findings to user to discuss which feedback to act on
- Run `/fix-findings` on the findings the user accepts
- Name the two or three idiomatic Go choices this feature made that are worth the user's understanding — the project is a learning vehicle (project.md §Why it exists)

### 6. Landing the plane

When the user says "land the plane":
- Re-run `/preflight` when any code changed after the gate (fix-findings edits, hand fixes)
- Move the plan to done: `git mv agent-context/plans/in-progress/<plan-name> agent-context/plans/done/<plan-name>`
- Clean up worktrees from the session
- Commit & push

### Error handling

- **Agent fails a task:** Surface the error and acceptance criteria to the user. Ask whether to retry, skip, or abort.
- **Merge conflict from concurrent agents:** Resolve if straightforward; escalate to user if the conflict involves architectural decisions.
- **Preflight fails:** Fix if the issue is mechanical (formatting, simple staticcheck lint). Escalate if the fix requires design decisions.

### Principles

- **You coordinate, you don't produce.** Every tool call spent building is context not spent orchestrating.
- **Guard context.** Each agent gets minimum viable context for their task.
- **3 of 4 completing is enough.** Partial progress with clear status beats blocking on one stuck task.
- **Surface, don't guess.** Tell the user when something unexpected happens. Don't make architectural decisions on their behalf.
