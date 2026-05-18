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
- The reporter and tracker write only to stderr. No progress output
  touches stdout.

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
      same interval with the same four fields (in_flight, done, total,
      elapsed_s). No `\r` or ANSI escapes appear in the output.
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
`time.Ticker`, calls `Snapshot`, and emits one line via a
`progressEmitter`. Mode (TTY vs. plain) is decided once at construction
by picking the emitter implementation.

```go
// Proposed design
type progressEmitter interface {
    emit(inFlight, done, total int, elapsed time.Duration)
    finalLine()
}
```

Both implementations live in `progress.go`:

- `ttyEmitter` wraps an `io.Writer` (typically `os.Stderr`). `emit`
  writes `"\r\x1b[K[batch-enrich] progress in_flight=%d done=%d/%d elapsed_s=%d"`
  with no trailing newline. `finalLine` writes `"\n"` so the cursor
  lands on a fresh line before the report or any closing log line.
  Between ticks, interleaved slog lines push the progress line up the
  scrollback; this is intentional and not a defect.
- `plainEmitter` wraps a `*slog.Logger`. `emit` calls
  `logger.Info("[batch-enrich] progress", "in_flight", ..., "done", ..., "total", ..., "elapsed_s", ...)`.
  `finalLine` is a no-op.

`Reporter` holds a single `progressEmitter` chosen at construction.

TTY detection uses `golang.org/x/term.IsTerminal(int(os.Stderr.Fd()))`.
`golang.org/x/term` is not currently in `go.mod`. Add it with
`go get golang.org/x/term` and commit the updated `go.mod`/`go.sum`.

Wiring in `cmd/batch-enrich/main.go`:

1. After `StripBoilerplate` returns `stripped`, compute total batches
   by walking the same wave/batch arithmetic the wave loop uses:
   `sum over waves of ceil(min(WaveSize, remaining) / BatchSize)`.
   Extract this into a helper in `cmd/batch-enrich/progress.go` so
   the wave loop and the total pre-calculation cannot drift:

   ```go
   // countBatches returns the total number of classifyBatch calls a run will make.
   func countBatches(totalPostings, waveSize, batchSize int) int
   ```

   The wave loop in `main.go` is refactored to use `countBatches` for
   the pre-calc; the loop's own arithmetic is unchanged (it still
   slices by `waveSize`) — the two share the same formula, so they
   cannot drift.
2. Construct `ProgressTracker` and `Reporter`. If
   `cfg.ProgressInterval > 0` (see Config additions below), derive
   `reporterCtx` from `ctx`, launch `go reporter.Run(reporterCtx)`,
   and track the goroutine with a `sync.WaitGroup`. `Reporter.Run`
   blocks on its ticker until `reporterCtx.Done()`, then calls the
   emitter's `finalLine` (TTY writes `"\n"`; plain is a no-op) and
   returns.
3. Pass the tracker into `RunWave` as a new trailing parameter:
   `tracker *ProgressTracker`. Nil is a no-op.
4. After the wave loop, on the normal-exit path (after the `ctx.Err()`
   guard in main.go that follows the wave loop): cancel `reporterCtx`,
   `wg.Wait()` for the reporter to drain, then emit a final summary
   line — TTY mode lands on a clean line and plain mode gets a `done`
   slog Info line.

   On the cancellation path, the two mid-wave exit points that
   `return 130` do not emit the final summary. `reporterCtx` is derived
   from `ctx`, so it is already cancelled — the reporter stops ticking
   on its own and runs its `finalLine` cleanup as it returns.

Wiring in `cmd/batch-enrich/dispatch.go`:

- `RunWave` accepts a `*ProgressTracker` (nil-safe: a nil tracker is a
  no-op so tests that don't care about progress can pass nil).
- Inside the per-batch goroutine, after the semaphore is acquired and
  before `classifyBatch` runs, call `tracker.BatchStarted()`. In a
  `defer`, call `tracker.BatchFinished()`. The semaphore-cancellation
  path (early return when `ctx.Done()` fires before slot acquisition)
  does **not** call `BatchStarted` — those batches never ran.
  The existing panic-recovery defer in dispatch.go is registered before
  semaphore acquisition; `defer tracker.BatchFinished()` is registered
  after, so LIFO ordering ensures `BatchFinished` fires before the
  recovery handler on a panic — the counters stay consistent.

Config additions in `cmd/batch-enrich/config.go`:

- `ProgressInterval time.Duration` with flag
  `--progress-interval` (default `2s`).
- Validation: `ProgressInterval >= 0`.
- Include in the startup log line as key `progress_interval`.

## Boundary inventory

Not applicable — change is internal to `cmd/batch-enrich`. No JSON, HTTP,
or SQL boundaries are crossed.

## Open questions

- Default interval of `2s` is a guess. If runs are short (single wave,
  a handful of batches), 2s may emit only one or two ticks. Acceptable —
  the operator can tighten via flag.
