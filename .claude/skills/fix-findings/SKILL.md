---
name: fix-findings
description: >
  Acts on review panel findings by dispatching concurrent Sonnet agents for
  small-blast-radius items (one per file), then an Opus agent for remaining
  issues with knock-on effects. All agents read relevant agent-context files.
  Use after /review-panel produces findings.
allowed-tools: Read, Glob, Grep, Bash, Agent
argument-hint: "[findings to act on — defaults to all reds and yellows]"
---

# Fix Findings

Triage review panel findings and dispatch agents to fix them. Coordinate — don't produce.

## Agent brief (provide to every agent)

- The specific findings to address: `file:line`, the panel's summary and detail, and your triage note on the expected fix direction (the panel reports problems, not fixes — you author the direction during triage)
- Read `agent-context/lib/index.md` and any files the router points to for the relevant area
- Read `agent-context/lib/style-guide.md` before updating any comments or docs
- Read `agent-context/lib/developer-guide.md` before writing code
- Confirm any claim about an identifier's shape or behavior by opening the file this session — never from memory
- Run `go build ./...` and `go test ./...` from `apps/tools/` before considering the task done

## Process

### 1. Verify

Panel findings include false positives — misread code, already-handled cases, invented problems. Reviewer models can't distinguish "I remember this" from "I verified this." Before any fix agent spawns, confirm each finding against source: open the file, check the claim.

- Confirmed → triage it.
- Refuted or already handled → drop it; note the drop in the report.
- Uncertain after reading the source → surface to the user. Never dispatch a fix for an unconfirmed finding.

Red and yellow code findings from the workflow-backed `/review-panel` arrive refutation-checked — spot-check them rather than re-verifying. Greens, comment-drift findings, and findings from any other source get the full pass.

### 2. Triage

Scope: red and yellow findings, code and comment-drift both — unless the caller passed a subset (the findings the user accepted). Greens: fix only when trivial and in a file already being fixed; otherwise leave them.

Classify each confirmed finding:

**Small blast radius** — Sonnet, concurrent:
- Confined to a single file
- No interface or contract changes
- No knock-on effects in other packages
- Examples: missing error handling, nit, stale comment, dead code

**Everything else** — Opus, sequential:
- Crosses file or package boundaries
- Interface, contract, or exported type changes
- Knock-on effects likely
- Requires architectural judgment

Group small findings by file. Each file gets one agent.

### 3. Sonnet agents (parallel)

Spawn one agent per file in a single message. Provide the agent brief above.

### 4. Wait and assess

Review outputs. Note unresolved findings.

### 5. Opus agents (sequential)

Spawn 1–2 agents, one at a time. Provide the agent brief, plus an enumeration of likely knock-on targets.

### 6. Report

What was fixed, what was dropped as refuted or unconfirmed, what was skipped and why, and whether `go build ./...` and `go test ./...` pass.
