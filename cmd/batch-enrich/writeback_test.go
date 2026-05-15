package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestIsRetryable_ClassifiesTransientErrors exercises the retry classifier
// against the error shapes writeback actually encounters: pgconn.PgError
// codes, EOF on a half-open connection, and the long-tail non-transient
// cases that must short-circuit.
func TestIsRetryable_ClassifiesTransientErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"serialization_failure", &pgconn.PgError{Code: "40001"}, true},
		{"deadlock_detected", &pgconn.PgError{Code: "40P01"}, true},
		{"unique_violation", &pgconn.PgError{Code: "23505"}, false},
		{"not_null_violation", &pgconn.PgError{Code: "23502"}, false},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"context.Canceled", context.Canceled, false},
		{"plain error", errors.New("boom"), false},
		// Wrapped PgError must still be detected via errors.As.
		{"wrapped serialization", errWrap(&pgconn.PgError{Code: "40001"}), true},
		{"wrapped unique_violation", errWrap(&pgconn.PgError{Code: "23505"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.err); got != tc.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// errWrap wraps an error so tests can verify errors.As-style unwrapping.
func errWrap(err error) error {
	return &wrappedErr{err: err}
}

type wrappedErr struct{ err error }

func (w *wrappedErr) Error() string { return "wrapped: " + w.err.Error() }
func (w *wrappedErr) Unwrap() error { return w.err }

// TestRetryTransient_RetriesOnTransientError verifies the retry-loop
// contract that writeOne relies on: a transient failure followed by a
// success returns nil with exactly the expected number of attempts, while
// a non-retryable error short-circuits on the first attempt.
//
// Note on backoff: the retry loop sleeps writeOneBackoffs (50ms, 100ms)
// before retries 2 and 3. The total wall time for a 2-attempt success path
// is ~50ms, well under any reasonable test timeout, so we don't stub the
// timer.
func TestRetryTransient_RetriesOnTransientError(t *testing.T) {
	t.Run("transient then success", func(t *testing.T) {
		calls := 0
		err := retryTransient(context.Background(), 1, func() error {
			calls++
			if calls == 1 {
				return &pgconn.PgError{Code: "40001"}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("expected nil error after retry, got %v", err)
		}
		if calls != 2 {
			t.Errorf("expected fn to be called 2 times, got %d", calls)
		}
	})

	t.Run("non-retryable returns immediately", func(t *testing.T) {
		calls := 0
		want := &pgconn.PgError{Code: "23505"} // unique_violation
		err := retryTransient(context.Background(), 1, func() error {
			calls++
			return want
		})
		if !errors.Is(err, want) {
			t.Errorf("expected unique_violation error, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected fn to be called 1 time (no retry), got %d", calls)
		}
	})

	t.Run("exhausts attempts on persistent transient error", func(t *testing.T) {
		calls := 0
		err := retryTransient(context.Background(), 1, func() error {
			calls++
			return &pgconn.PgError{Code: "40P01"}
		})
		if err == nil {
			t.Fatalf("expected error after exhausting retries")
		}
		if calls != writeOneMaxAttempts {
			t.Errorf("expected fn to be called %d times, got %d", writeOneMaxAttempts, calls)
		}
	})

	t.Run("cancelled context aborts during backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		err := retryTransient(ctx, 1, func() error {
			calls++
			// Cancel before returning so the backoff select hits ctx.Done().
			cancel()
			return &pgconn.PgError{Code: "40001"}
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
		if calls != 1 {
			t.Errorf("expected fn to be called 1 time before cancellation, got %d", calls)
		}
	})
}
