package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

func TestClassifyCompanyError(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		parentCtxErr error
		wantAborted  bool
	}{
		{
			name:         "canceled with parent done → aborted_shutdown",
			err:          context.Canceled,
			parentCtxErr: context.Canceled,
			wantAborted:  true,
		},
		{
			name:         "canceled without parent done → failed (per-company cancel, not shutdown)",
			err:          context.Canceled,
			parentCtxErr: nil,
			wantAborted:  false,
		},
		{
			name:         "deadline exceeded → failed (per-company timeout, not shutdown)",
			err:          context.DeadlineExceeded,
			parentCtxErr: context.Canceled,
			wantAborted:  false,
		},
		{
			name:         "generic error → failed",
			err:          errors.New("some db error"),
			parentCtxErr: nil,
			wantAborted:  false,
		},
		{
			name:         "wrapped canceled with parent done → aborted_shutdown",
			err:          fmt.Errorf("fetching postings: %w", context.Canceled),
			parentCtxErr: context.Canceled,
			wantAborted:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCompanyError(tc.err, tc.parentCtxErr)
			if got != tc.wantAborted {
				t.Errorf("classifyCompanyError(%v, %v) = %v, want %v", tc.err, tc.parentCtxErr, got, tc.wantAborted)
			}
		})
	}
}

func TestValidatePosting(t *testing.T) {
	validRaw := json.RawMessage(`{"id":1}`)
	title := "Role"

	t.Run("valid posting passes", func(t *testing.T) {
		p := domain.Posting{
			SourceID:  "123",
			SourceURL: "https://example.com/jobs/123",
			Title:     &title,
			RawData:   validRaw,
		}
		if err := validatePosting(0, p); err != nil {
			t.Errorf("validatePosting: unexpected error: %v", err)
		}
	})

	t.Run("empty SourceURL is rejected", func(t *testing.T) {
		p := domain.Posting{SourceID: "123", RawData: validRaw}
		if err := validatePosting(0, p); err == nil {
			t.Error("validatePosting: expected error for empty SourceURL, got nil")
		}
	})

	t.Run("empty SourceID is rejected", func(t *testing.T) {
		p := domain.Posting{SourceURL: "https://example.com/jobs/123", RawData: validRaw}
		if err := validatePosting(0, p); err == nil {
			t.Error("validatePosting: expected error for empty SourceID, got nil")
		}
	})

	t.Run("empty RawData is rejected", func(t *testing.T) {
		p := domain.Posting{
			SourceID:  "123",
			SourceURL: "https://example.com/jobs/123",
			RawData:   nil,
		}
		if err := validatePosting(0, p); err == nil {
			t.Error("validatePosting: expected error for empty RawData, got nil")
		}
	})

	t.Run("error message includes posting index", func(t *testing.T) {
		p := domain.Posting{SourceID: "123", RawData: validRaw} // empty SourceURL
		err := validatePosting(3, p)
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		if !strings.Contains(err.Error(), "3") {
			t.Errorf("error %q does not include posting index 3", err.Error())
		}
	})
}

func TestSnapshotWrite_ForwardsSourceTimestampsToParams(t *testing.T) {
	// Both ATS-reported timestamps set on the domain.Posting must arrive in
	// InsertPostingSnapshotParams as sql.NullTime{Valid: true} carrying the
	// exact instant set on the domain field. This pins the wiring through
	// nullTime so a future refactor can't silently drop one of the new columns.
	first := time.Date(2026, 4, 17, 12, 21, 54, 0, time.UTC)
	last := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	p := domain.Posting{
		SourceID:               "123",
		SourceURL:              "https://example.com/jobs/123",
		RawData:                json.RawMessage(`{"id":123}`),
		SourceFirstPublishedAt: &first,
		SourceLastModifiedAt:   &last,
	}
	fetchedAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	got := buildSnapshotParams(99, fetchedAt, p)

	wantFirst := sql.NullTime{Time: first, Valid: true}
	if got.SourceFirstPublishedAt != wantFirst {
		t.Errorf("SourceFirstPublishedAt: got %+v, want %+v", got.SourceFirstPublishedAt, wantFirst)
	}
	wantLast := sql.NullTime{Time: last, Valid: true}
	if got.SourceLastModifiedAt != wantLast {
		t.Errorf("SourceLastModifiedAt: got %+v, want %+v", got.SourceLastModifiedAt, wantLast)
	}
	if got.JobPostingID != 99 {
		t.Errorf("JobPostingID: got %d, want 99", got.JobPostingID)
	}
	if !got.FetchedAt.Equal(fetchedAt) {
		t.Errorf("FetchedAt: got %v, want %v", got.FetchedAt, fetchedAt)
	}
}

func TestSnapshotWrite_PreservesNonUTCTimezoneOffset(t *testing.T) {
	// Greenhouse emits timestamps with non-UTC offsets (e.g. -04:00). The
	// nullTime conversion must preserve the original Location so callers can
	// observe the ATS-reported offset. A silent .UTC() normalization would
	// represent the same instant but discard the offset, losing the signal.
	edt := time.FixedZone("EDT", -4*3600)
	first := time.Date(2026, 4, 17, 12, 21, 54, 0, edt)
	last := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	p := domain.Posting{
		SourceID:               "456",
		SourceURL:              "https://example.com/jobs/456",
		RawData:                json.RawMessage(`{"id":456}`),
		SourceFirstPublishedAt: &first,
		SourceLastModifiedAt:   &last,
	}
	got := buildSnapshotParams(1, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), p)

	if !got.SourceFirstPublishedAt.Valid {
		t.Fatal("SourceFirstPublishedAt: got Valid=false, want Valid=true")
	}
	if !got.SourceFirstPublishedAt.Time.Equal(first) {
		t.Errorf("SourceFirstPublishedAt: instant mismatch: got %v, want %v",
			got.SourceFirstPublishedAt.Time, first)
	}
	if got.SourceFirstPublishedAt.Time.Location().String() != edt.String() {
		t.Errorf("SourceFirstPublishedAt: location not preserved: got %q, want %q",
			got.SourceFirstPublishedAt.Time.Location().String(), edt.String())
	}
}

func TestSnapshotWrite_NilSourceTimestampsBecomeNullTime(t *testing.T) {
	// nil pointers on the domain side must produce zero-value sql.NullTime
	// (Valid: false) — the column persists as NULL.
	p := domain.Posting{
		SourceID:  "123",
		SourceURL: "https://example.com/jobs/123",
		RawData:   json.RawMessage(`{"id":123}`),
	}
	got := buildSnapshotParams(1, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), p)
	if got.SourceFirstPublishedAt.Valid {
		t.Errorf("SourceFirstPublishedAt: got Valid=true, want Valid=false for nil input")
	}
	if got.SourceLastModifiedAt.Valid {
		t.Errorf("SourceLastModifiedAt: got Valid=true, want Valid=false for nil input")
	}
}

func TestSummaryInvariant(t *testing.T) {
	// The invariant success + failed + aborted_shutdown = total_attempted must
	// hold for every possible run outcome. Test the arithmetic across all
	// combinations that can arise from the dispatch loop.
	cases := []struct {
		name           string
		success        int
		failed         int
		aborted        int
		totalAttempted int
		wantViolated   bool
	}{
		{"all success", 5, 0, 0, 5, false},
		{"all failed", 0, 5, 0, 5, false},
		{"all aborted", 0, 0, 5, 5, false},
		{"mixed", 2, 2, 1, 5, false},
		{"undercounted — goroutine dropped outcome", 2, 2, 0, 5, true},
		{"overcounted — goroutine double-reported", 3, 2, 1, 5, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violated := tc.success+tc.failed+tc.aborted != tc.totalAttempted
			if violated != tc.wantViolated {
				t.Errorf("invariant check: got violated=%v, want %v (success=%d failed=%d aborted=%d total=%d)",
					violated, tc.wantViolated, tc.success, tc.failed, tc.aborted, tc.totalAttempted)
			}
		})
	}
}
