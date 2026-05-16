# Batch Enrich Progress Reporting

## Goal

Give the operator continuous feedback while `cmd/batch-enrich` is running:
how many agent batches are in flight, how many have completed, and how long
the run has been going. Removes the long "did it hang?" gap between
selection and the final report.

## Scope

### In scope

- A progress tracker, shared across all waves of a run, that counts agent
  batches (one unit per `classifyBatch` invocation).
- A reporter goroutine that emits progress to stderr on a fixed interval.
- TTY mode: single line at the bottom of the terminal, refreshed in place
  (carriage-return + clear-to-EOL). Interleaved slog lines scroll past;
  progress always reappears on the next tick.
- Non-TTY mode (pipes, CI, log files): one slog Info line per tick. No
  control characters.
- A final "done" summary line when the wave loop exits, before the report
  is emitted to stdout.
- `--progress-interval` flag, default `2s`. `0` disables progress output.

### Out of scope

- Boilerplate-stripping progress. `StripBoilerplate` runs once up front
  and emits its own slog line; not worth wiring through the tracker.
- Writeback progress. `WriteBack` already logs per-classification at Info
  level; no separate counter.
- Per-posting progress within a batch. Batches are the natural unit
  because `MaxParallelAgents` bounds batched calls, not postings.
- ETA / rate estimation. Elapsed seconds only.
- Counting retries as separate batches. Retries live inside one
  `classifyBatch` call and are invisible to the tracker.
- Structured progress events on a separate stream (JSON lines, metrics).

## Acceptance criteria

- [ ] During a multi-batch run on a TTY, stderr shows a single progress
      line that updates at least once per `--progress-interval` and
      reports in-flight count, completed/total batches, and elapsed
      whole seconds since run start.
- [ ] When stderr is not a TTY (piped, redirected, non-interactive
      shell), progress is emitted as discrete slog Info lines on the
      same interval with the same three fields. No `\r` or ANSI escapes
      appear in the output.
- [ ] `--progress-interval=0` produces no progress output in either
      mode; all other run output is unchanged.
- [ ] In-flight count never exceeds `--max-parallel`.
- [ ] Completed count equals total batches when the wave loop finishes
      normally. Total batches matches the number of `classifyBatch`
      calls the run actually makes.
- [ ] On SIGINT / SIGTERM the reporter stops cleanly without printing
      after the cancellation message; the existing exit-130 path is
      unchanged.
- [ ] Retries inside `classifyBatch` do not advance the completed
      counter or change the in-flight count.
- [ ] The report on stdout is byte-identical to the pre-change output
      for an equivalent run (progress goes to stderr only).

## Rough sketch

New file `cmd/batch-enrich/progress.go` holding:

```go
// Proposed design
type ProgressTracker struct {
    total     int
    done      atomic.Int64
    inFlight  atomic.Int64
    startedAt time.Time
}

func (p *ProgressTracker) BatchStarted()  // inFlight++
func (p *ProgressTracker) BatchFinished() // inFlight--, done++
func (p *ProgressTracker) Snapshot() (inFlight, done, total int, elapsed time.Duration)
```

Reporter is a separate type with a `Run(ctx)` method that loops on a
`time.Ticker`, calls `Snapshot`, and writes one line via an injected
writer. Mode (TTY vs. plain) is decided once at construction:

- TTY writer: `fmt.Fprintf(w, "\r\x1b[K[batch-enrich] progress in_flight=%d done=%d/%d elapsed_s=%d", ...)`.
  No trailing newline. A final newline is written when the reporter stops
  so the cursor lands on a fresh line before the report or any closing
  log line.
- Plain writer: `slog.Info("[batch-enrich] progress", "in_flight", ..., "done", ..., "total", ..., "elapsed_s", ...)`.

TTY detection uses `golang.org/x/term.IsTerminal(int(os.Stderr.Fd()))`.
`golang.org/x/term` is already a transitive dep via the Go toolchain;
add it to `go.mod` directly if not.

Wiring in `cmd/batch-enrich/main.go`:

1. After `StripBoilerplate` returns `stripped`, compute total batches
   by walking the same wave/batch arithmetic the wave loop uses:
   `sum over waves of ceil(min(WaveSize, remaining) / BatchSize)`.
   Extract this into a helper so the wave loop and the total
   pre-calculation cannot drift.
2. Construct `ProgressTracker` and `Reporter`. If
   `cfg.ProgressInterval > 0`, start the reporter in a goroutine with
   a context derived from `ctx`.
3. Pass the tracker into `RunWave` as a new parameter (or as a field
   on a small dispatch struct — implementor's call).
4. After the wave loop, stop the reporter (cancel its context, wait
   for it to drain), then emit a final summary line through the same
   writer so TTY mode lands on a clean line and plain mode gets a
   `done` slog Info line.

Wiring in `cmd/batch-enrich/dispatch.go`:

- `RunWave` accepts a `*ProgressTracker` (nil-safe: a nil tracker is a
  no-op so tests that don't care about progress can pass nil).
- Inside the per-batch goroutine, after the semaphore is acquired and
  before `classifyBatch` runs, call `tracker.BatchStarted()`. In a
  `defer`, call `tracker.BatchFinished()`. The semaphore-cancellation
  path (early return when `ctx.Done()` fires before slot acquisition)
  does **not** call `BatchStarted` — those batches never ran.

Config additions in `cmd/batch-enrich/config.go`:

- `ProgressInterval time.Duration` with flag
  `--progress-interval` (default `2s`).
- Validation: `ProgressInterval >= 0`.
- Include in the startup log line.

## Boundary inventory

Not applicable — change is internal to `cmd/batch-enrich`. No JSON, HTTP,
or SQL boundaries are crossed.

## Open questions

- Should the TTY progress line include a count of postings (e.g.
  `postings=37/200`) in addition to batches? Adds a second derived
  number but is arguably what a human reads first. Default: batches
  only, matches the unit decision above. Revisit if the line feels
  unhelpful in practice.
- Default interval of `2s` is a guess. If runs are short (single wave,
  a handful of batches), 2s may emit only one or two ticks. Acceptable —
  the operator can tighten via flag.
