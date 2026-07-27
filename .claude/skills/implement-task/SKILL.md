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

Read carefully:
- `agent-context/lib/developer-guide.md` — conventions, constraints, coding standards
- `agent-context/lib/testing-guide.md` — what to test, test patterns

Read `agent-context/lib/index.md`; use the router to load only the files this task needs.

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

From `apps/tools/`:
- `go build ./...` and `go vet ./...` — free, run freely
- `go test ./<touched-package> -count=1` while iterating, then `go test ./...`
- `gofmt -l .` — fix anything listed

Free tier only. Never run live-surface commands: `cmd/fetcher` hits live ATS APIs, `cmd/batch-enrich` spends money per posting. Those belong to the coordinator or the human (developer-guide §2, Cost map).

### 6. Report

Return exactly this:
- Each acceptance criterion with a status: met, not met, or deviated
- Verbatim `go build ./...` and `go test ./...` output — not "tests pass"
- **Deviations, guesses, and assumptions** — every departure from the task and every unverified assumption. An empty section asserts "everything I built was verified against source."
- Files changed

Do not commit, push, or run `/preflight`. The caller handles integration.
