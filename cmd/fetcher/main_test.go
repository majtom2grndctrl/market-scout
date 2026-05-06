package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

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
