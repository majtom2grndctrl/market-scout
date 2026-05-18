// Progress reporting for batch-enrich runs. Owns ProgressTracker (shared
// batch counters), Reporter (fixed-interval emitter), and countBatches
// (the batch-total formula shared with the dispatch loop).
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// ProgressTracker counts agent batches across all waves of a run. All
// methods are safe for concurrent use. A nil *ProgressTracker is a valid
// no-op — RunWave calls the BatchStarted/BatchFinished methods through
// nil-safe wrappers so test paths that don't care about progress don't
// have to construct one.
type ProgressTracker struct {
	total     int
	done      atomic.Int64
	inFlight  atomic.Int64
	startedAt time.Time
}

// BatchStarted records that an agent batch has begun (after semaphore
// acquisition). Safe to call on a nil receiver.
func (p *ProgressTracker) BatchStarted() {
	if p == nil {
		return
	}
	p.inFlight.Add(1)
}

// BatchFinished records that an agent batch has finished. Decrements
// in-flight and increments done. Safe to call on a nil receiver.
func (p *ProgressTracker) BatchFinished() {
	if p == nil {
		return
	}
	p.done.Add(1)
	p.inFlight.Add(-1)
}

// Snapshot returns a point-in-time view of the counters plus elapsed time
// since the tracker was constructed. The three int values are reads of
// atomic counters taken in succession; they may not represent a single
// instantaneous state, but the reporter only uses them for operator-facing
// display so the slight skew is acceptable.
func (p *ProgressTracker) Snapshot() (inFlight, done, total int, elapsed time.Duration) {
	if p == nil {
		return 0, 0, 0, 0
	}
	return int(p.inFlight.Load()), int(p.done.Load()), p.total, time.Since(p.startedAt)
}

// progressEmitter is the interface a Reporter writes through. The two
// implementations differ in how a tick is rendered (single-line carriage
// return for TTY, discrete slog Info lines for non-TTY) and whether a
// final newline is needed to leave the cursor on its own line.
type progressEmitter interface {
	emit(inFlight, done, total int, elapsed time.Duration)
	finalLine()
	emitSummary(totalBatches int, elapsed time.Duration)
}

// ttyEmitter renders progress as a single-line, carriage-return-prefixed
// update on an io.Writer (typically os.Stderr). The ANSI clear-line escape
// ("\x1b[K") avoids leaving stale tail characters when a longer line is
// overwritten by a shorter one.
type ttyEmitter struct {
	w io.Writer
}

func (e *ttyEmitter) emit(inFlight, done, total int, elapsed time.Duration) {
	fmt.Fprintf(e.w, "\r\x1b[K[batch-enrich] progress in_flight=%d done=%d/%d elapsed_s=%d",
		inFlight, done, total, int(elapsed.Seconds()))
}

func (e *ttyEmitter) finalLine() {
	fmt.Fprint(e.w, "\n")
}

func (e *ttyEmitter) emitSummary(totalBatches int, elapsed time.Duration) {
	fmt.Fprintf(e.w, "[batch-enrich] done: classified %d batches in %ds\n", totalBatches, int(elapsed.Seconds()))
}

// plainEmitter renders progress as discrete structured log lines. Used
// when stderr is not a TTY (CI, log redirection, etc.) so output stays
// grep-friendly and free of escape sequences.
type plainEmitter struct {
	logger *slog.Logger
}

func (e *plainEmitter) emit(inFlight, done, total int, elapsed time.Duration) {
	e.logger.Info("[batch-enrich] progress",
		"in_flight", inFlight,
		"done", done,
		"total", total,
		"elapsed_s", int(elapsed.Seconds()),
	)
}

func (e *plainEmitter) finalLine() {}

func (e *plainEmitter) emitSummary(totalBatches int, elapsed time.Duration) {
	e.logger.Info("[batch-enrich] done", "batches", totalBatches, "elapsed_s", int(elapsed.Seconds()))
}

// Reporter emits progress on a fixed interval until its context is
// cancelled. Construct one via NewReporter and run it as a goroutine
// alongside the wave loop.
type Reporter struct {
	tracker  *ProgressTracker
	interval time.Duration
	emitter  progressEmitter
}

// NewReporter picks a TTY or plain emitter based on whether stderr is a
// terminal. The caller owns the lifetime of stderr; Reporter never closes
// it.
func NewReporter(tracker *ProgressTracker, interval time.Duration, stderrFile *os.File) *Reporter {
	var emitter progressEmitter
	if stderrFile != nil && term.IsTerminal(int(stderrFile.Fd())) {
		emitter = &ttyEmitter{w: stderrFile}
	} else {
		emitter = &plainEmitter{logger: slog.Default()}
	}
	return &Reporter{
		tracker:  tracker,
		interval: interval,
		emitter:  emitter,
	}
}

// newReporterWithEmitter constructs a Reporter with an explicit emitter
// for tests; production code uses NewReporter to get TTY-aware emitter
// selection.
func newReporterWithEmitter(tracker *ProgressTracker, interval time.Duration, emitter progressEmitter) *Reporter {
	return &Reporter{tracker: tracker, interval: interval, emitter: emitter}
}

// Run blocks until ctx is cancelled. On each tick it pulls a snapshot from
// the tracker and hands it to the emitter; on cancellation it calls
// finalLine so the next stderr write (the summary, or a shell prompt)
// starts on its own line. finalLine is skipped if no tick fired, so a
// ttyEmitter does not write a bare newline when the run finishes before
// the first interval elapses.
func (r *Reporter) Run(ctx context.Context) {
	if r.interval <= 0 {
		return
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	var emitted bool
	for {
		select {
		case <-ctx.Done():
			if emitted {
				r.emitter.finalLine()
			}
			return
		case <-ticker.C:
			inFlight, done, total, elapsed := r.tracker.Snapshot()
			r.emitter.emit(inFlight, done, total, elapsed)
			emitted = true
		}
	}
}

// EmitSummary writes the final "done" line through the same emitter the
// periodic ticks use, so the summary's format (TTY plain text vs. structured
// slog line) stays in lock-step with the ticks even if stderr is rewired.
// The dedicated emitSummary path uses keys ("batches", "elapsed_s") distinct
// from the in-progress tick fields so log consumers can tell the terminal
// summary apart from mid-run ticks.
func (r *Reporter) EmitSummary(totalBatches int, elapsed time.Duration) {
	r.emitter.emitSummary(totalBatches, elapsed)
}

// countBatches returns the total number of classifyBatch calls a run will
// make given totalPostings, waveSize, and batchSize. Used both for the
// pre-calculation in main.go and to keep the pre-calculation and the wave
// loop from drifting apart: the formula is sum over waves of
// ceil(min(waveSize, remaining) / batchSize).
func countBatches(totalPostings, waveSize, batchSize int) int {
	if totalPostings <= 0 || waveSize <= 0 || batchSize <= 0 {
		return 0
	}
	total := 0
	remaining := totalPostings
	for remaining > 0 {
		wave := waveSize
		if wave > remaining {
			wave = remaining
		}
		// ceil(wave / batchSize)
		total += (wave + batchSize - 1) / batchSize
		remaining -= wave
	}
	return total
}
