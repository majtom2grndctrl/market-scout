package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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
	emits     int32
	finalized int32
	lastDone  int
	lastTotal int
}

func (f *fakeEmitter) emit(inFlight, done, total int, elapsed time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	atomic.AddInt32(&f.emits, 1)
	f.lastDone = done
	f.lastTotal = total
}

func (f *fakeEmitter) finalLine() {
	atomic.AddInt32(&f.finalized, 1)
}

func TestReporter_TicksAndStopsOnCancel(t *testing.T) {
	tracker := &ProgressTracker{total: 5, startedAt: time.Now()}
	emitter := &fakeEmitter{}
	reporter := newReporterWithEmitter(tracker, 5*time.Millisecond, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reporter.Run(ctx)
		close(done)
	}()

	// Let the ticker fire a few times.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("reporter did not return after cancellation")
	}

	if got := atomic.LoadInt32(&emitter.emits); got < 1 {
		t.Errorf("expected at least 1 emit, got %d", got)
	}
	if got := atomic.LoadInt32(&emitter.finalized); got != 1 {
		t.Errorf("expected finalLine to be called once, got %d", got)
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
	_ = RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
}
