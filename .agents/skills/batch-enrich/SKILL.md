---
name: batch-enrich
description: Run a stated batch-enrichment job through the Go command.
argument-hint: "[count] [focus description] [--force]"
allowed-tools: Bash
---

# Batch Enrich

Run one authorized batch through `apps/tools/cmd/batch-enrich`.

`cmd/batch-enrich` owns selection, classification, validation, retries,
writeback, and reporting. Do not reproduce those contracts here.

## Arguments

Normalize `$ARGUMENTS` before dispatch:

1. Split arguments into whitespace-delimited tokens.
2. Remove every token exactly equal to `--force`, wherever it appears. Record
   whether one or more were removed.
3. If no tokens remain, use count `10` and an empty focus value.
4. Otherwise, the first remaining token is the count. It must be decimal digits
   representing a value greater than zero. Invalid, zero, or negative values
   stop the run before dispatch.
5. Join all remaining tokens after the count with one space. Pass that string as
   one `--focus` value.

`--force` is the only recognized control token. A token that merely contains
that text is focus text or an invalid count, as determined above.

## Run

Invoking this skill with arguments authorizes exactly the normalized batch. Do
not ask for separate confirmation.

Set the shell working directory to `apps/tools/`. Build the command as an
argument array so focus stays one argument:

```bash
go run ./cmd/batch-enrich \
  --runner=codex-exec \
  --agent-timeout=5m \
  --count="$count" \
  --focus="$focus"
```

Append `--force` only when normalization removed at least one exact force token.

On success, surface the final `counts` from the command's stdout report and the
full stdout report. On failure, surface the command's stderr and stdout, then
stop.
