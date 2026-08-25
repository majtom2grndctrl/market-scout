---
name: implement-task
description: >
  Implements a single task from a plan or ad-hoc description. Reads the task,
  loads context via the agent router, examines dependent code, builds the
  feature, and returns a structured completion report. Use when working a
  specific task from a plan, or for a focused implementation request.
context: fork
allowed-tools: Read, Glob, Grep, Bash, Edit, Write
argument-hint: "[plan-name | task description]"
---

# Implement Task

Implement a single task. Read the spec, load context, understand dependencies, build. Do not commit — the caller handles integration.

Dispatched by `/orchestrate`? Your brief already inlines context and task — skip §1–§2 and start at §3.

## Process

### 1. Load context

Read `agent-context/lib/index.md` first. Route from the surface this task touches to the guides that govern it, and load only those:

| Surface | Guides |
|---|---|
| `apps/tools/` | `developer-guide.md`, `testing-guide.md` |
| `apps/web/` | `web-guide.md` — it names the few `developer-guide.md` sections that still apply |
| Both | Both sets |

Reading the other surface's guide is wasted context and invites its conventions into code they don't govern.

Code-grounding rule: any claim about an identifier's shape or behavior comes from a file opened this session, not memory.

### 2. Load the task

**Plan task:** read `agent-context/plans/in-progress/<plan-name>/index.md`. Extract the Goal section, your task section, and the acceptance criteria. Skip other tasks' text.

**Ad-hoc description:** the description is the spec. Ask clarifying questions only when acceptance criteria are ambiguous.

!`ls agent-context/plans/in-progress/ 2>/dev/null`

### 3. Examine dependencies

Before writing code:
- Read the code you'll modify or depend on
- Trace data contracts at package boundaries — what do consumers expect?
- Check related tests that document existing behavior
- If the task consumes another task's output, confirm that output exists in the tree

### 4. Implement

- Deliver the acceptance criteria. No more, no less.
- Follow developer-guide and testing-guide conventions. They help.
- Handle error states and degradation paths within scope.
- Never hand-edit sqlc output — change the `.sql` source, run `sqlc generate` (developer-guide §5.8).

### 5. Verify

Run your surface's checks from `.claude/skills/preflight/SKILL.md` — §Tools gate or §Web gate. Those tables are the single definition of what proves code sound here; don't carry a command list in your head.

While iterating, run the narrow fast check and skip the slow one. Before reporting, run the full set for your surface.

| Surface | Fast loop | Slow, before reporting |
|---|---|---|
| `apps/tools/` | `go build ./...`, `go vet ./...`, `go test ./<touched-package> -count=1` | `go test ./...`, `gofmt -l .` |
| `apps/web/` | `pnpm typecheck`, `pnpm test` | `pnpm build-storybook`, `pnpm build` |

Free tier only. Never run live-surface commands: `cmd/fetcher` hits live ATS APIs, `cmd/batch-enrich` spends money per posting. Those belong to the coordinator or the human (developer-guide §2, Cost map). `apps/web` has no paid surface today; when the chat surface lands, model calls become one.

### 6. Report

Return exactly this:
- Each acceptance criterion with a status: met, not met, or deviated
- Verbatim output of your surface's verify commands — not "tests pass". `go build` and `go test` on `tools`; `pnpm typecheck`, `pnpm test`, and `pnpm build-storybook` on `web`
- **Deviations, guesses, and assumptions** — every departure from the task and every unverified assumption. An empty section asserts "everything I built was verified against source."
- Files changed
- **Web only — what a reviewer should look at:** the stories and routes your change added or altered. Name them; don't assess them. A component that compiles can still render a chart that lies, and judging that is the coordinator's or the user's job, not yours.

Do not commit, push, or run `/preflight`. The caller handles integration.
