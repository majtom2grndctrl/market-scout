package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCountBatches(t *testing.T) {
	cases := []struct {
		name          string
		totalPostings int
		waveSize      int
		batchSize     int
		want          int
	}{
		{name: "zero postings", totalPostings: 0, waveSize: 10, batchSize: 5, want: 0},
		{name: "wave smaller than batch", totalPostings: 3, waveSize: 3, batchSize: 5, want: 1},
		{name: "wave larger than batch, even divide", totalPostings: 10, waveSize: 10, batchSize: 5, want: 2},
		{name: "wave larger than batch, remainder", totalPostings: 10, waveSize: 10, batchSize: 3, want: 4},
		{name: "two full waves", totalPostings: 20, waveSize: 10, batchSize: 5, want: 4},
		{name: "two waves, final partial", totalPostings: 13, waveSize: 10, batchSize: 5, want: 3},
		{name: "single posting", totalPostings: 1, waveSize: 10, batchSize: 5, want: 1},
		{name: "batch size 1", totalPostings: 7, waveSize: 10, batchSize: 1, want: 7},
		{name: "zero wave size guarded", totalPostings: 5, waveSize: 0, batchSize: 5, want: 0},
		{name: "zero batch size guarded", totalPostings: 5, waveSize: 5, batchSize: 0, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := countBatches(tc.totalPostings, tc.waveSize, tc.batchSize)
			if got != tc.want {
				t.Errorf("countBatches(%d, %d, %d) = %d, want %d",
					tc.totalPostings, tc.waveSize, tc.batchSize, got, tc.want)
			}
		})
	}
}

// TestProgressTracker_ConcurrentBatches exercises BatchStarted/BatchFinished
// from many goroutines and verifies that the final Snapshot lands at the
// expected zero-in-flight / done==N steady state. Without atomic counters
// this test would race; the race detector (`go test -race`) catches the
// regression.
func TestProgressTracker_ConcurrentBatches(t *testing.T) {
	tracker := &ProgressTracker{total: 100, startedAt: time.Now()}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.BatchStarted()
			tracker.BatchFinished()
		}()
	}
	wg.Wait()

	inFlight, done, total, elapsed := tracker.Snapshot()
	if inFlight != 0 {
		t.Errorf("inFlight: want 0, got %d", inFlight)
	}
	if done != 100 {
		t.Errorf("done: want 100, got %d", done)
	}
	if total != 100 {
		t.Errorf("total: want 100, got %d", total)
	}
	if elapsed < 0 {
		t.Errorf("elapsed must be non-negative, got %s", elapsed)
	}
}

// TestProgressTracker_NilReceiverIsNoop documents that a nil tracker is a
// valid no-op — RunWave's dispatch path calls these methods unconditionally,
// and tests that don't care about progress pass nil.
func TestProgressTracker_NilReceiverIsNoop(t *testing.T) {
	var tracker *ProgressTracker
	// These must not panic.
	tracker.BatchStarted()
	tracker.BatchFinished()
	inFlight, done, total, elapsed := tracker.Snapshot()
	if inFlight != 0 || done != 0 || total != 0 || elapsed != 0 {
		t.Errorf("nil Snapshot: want all-zero, got inFlight=%d done=%d total=%d elapsed=%s",
			inFlight, done, total, elapsed)
	}
}

// fakeEmitter captures emit/finalLine calls so Reporter behavior can be
// asserted without timing-sensitive stderr scraping.
type fakeEmitter struct {
	mu        sync.Mutex
	once      sync.Once
	firstEmit chan struct{}
	emits     int
	finalized int
	lastDone  int
	lastTotal int
}

func (f *fakeEmitter) emit(inFlight, done, total int, elapsed time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emits++
	f.lastDone = done
	f.lastTotal = total
	if f.firstEmit != nil {
		f.once.Do(func() { close(f.firstEmit) })
	}
}

func (f *fakeEmitter) finalLine() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finalized++
}

func (f *fakeEmitter) emitSummary(totalBatches int, elapsed time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastDone = totalBatches
	f.lastTotal = totalBatches
}

func TestReporter_TicksAndStopsOnCancel(t *testing.T) {
	tracker := &ProgressTracker{total: 5, startedAt: time.Now()}
	firstEmit := make(chan struct{})
	emitter := &fakeEmitter{firstEmit: firstEmit}
	reporter := newReporterWithEmitter(tracker, 5*time.Millisecond, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reporter.Run(ctx)
		close(done)
	}()

	// Wait until at least one tick fires, then cancel.
	select {
	case <-firstEmit:
	case <-time.After(time.Second):
		t.Fatalf("reporter did not emit after 1s")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("reporter did not return after cancellation")
	}

	emitter.mu.Lock()
	emits := emitter.emits
	finalized := emitter.finalized
	emitter.mu.Unlock()

	if emits < 1 {
		t.Errorf("expected at least 1 emit, got %d", emits)
	}
	if finalized != 1 {
		t.Errorf("expected finalLine to be called once, got %d", finalized)
	}
}

func TestReporter_ZeroIntervalReturnsImmediately(t *testing.T) {
	tracker := &ProgressTracker{total: 0, startedAt: time.Now()}
	emitter := &fakeEmitter{}
	reporter := newReporterWithEmitter(tracker, 0, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		reporter.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("reporter with zero interval did not return immediately")
	}
}

// TestRunWave_NilTrackerDoesNotPanic exercises the nil-tracker path through
// RunWave to confirm BatchStarted/BatchFinished are nil-safe under real
// dispatch flow, not just in isolation.
func TestRunWave_NilTrackerDoesNotPanic(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return "", "raw", errors.New("boom")
		},
	}

	// nil tracker passed through.
	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// TestRunWave_TrackerCountsCompletedBatches verifies that a non-nil tracker
// lands at done==1, inFlight==0 after a single-posting wave with batch size 1.
func TestRunWave_TrackerCountsCompletedBatches(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	tracker := &ProgressTracker{total: 1, startedAt: time.Now()}

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return "", "raw", errors.New("boom")
		},
	}

	_ = RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, tracker)

	_, done, _, _ := tracker.Snapshot()
	if done != 1 {
		t.Errorf("expected done=1 after 1-posting wave, got %d", done)
	}
	inFlight, _, _, _ := tracker.Snapshot()
	if inFlight != 0 {
		t.Errorf("expected inFlight=0 after wave, got %d", inFlight)
	}
}
