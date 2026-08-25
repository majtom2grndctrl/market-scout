---
name: batch-enrich
description: Run one authorized Market Scout batch-enrichment job through the Codex-backed Go command. Use when the user asks to classify a stated number of job postings, optionally with a focus or `--force`.
---

# Batch Enrich

Run the established command. It owns selection, classification, boilerplate stripping, validation, retries, provenance, writeback, and reporting. Do not reimplement those contracts in the skill.

## Arguments

Parse the request as `<count> [focus] [--force]`.

- Remove exact `--force` tokens before parsing. A word containing `force` remains focus text.
- Default to count `10` and an empty focus when count is absent.
- Reject zero, negative, and non-numeric counts before dispatch.
- Join remaining focus words into one `--focus` argument.

## Run

The user's invocation authorizes exactly the normalized batch. Do not ask again.

Run from `apps/tools/`:

```bash
go run ./cmd/batch-enrich \
  --runner=codex-exec \
  --agent-timeout=5m \
  --count="$count" \
  --focus="$focus"
```

Append `--force` only when supplied. Surface the command's complete report on success. On failure, surface stdout and stderr, then stop.

## Rules

- Treat this as an operator action, never verification.
- Do not use `--force` to re-verify code. It spends money and adds provenance history.
- Do not replace the command with direct MCP writes or delegated classification agents.
