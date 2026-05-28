package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// fakeStripRunner is a programmable stripRunner for boilerplate tests. The
// responder closure is invoked once per company; tests can model "fails on
// second company", "returns empty cleaned_text", etc. without sharing state.
type fakeStripRunner struct {
	calls     int32
	responder func(companyID int64, selectedIDs []int64) ([]stripPosting, error)
}

func (f *fakeStripRunner) Run(_ context.Context, companyID int64, selectedIDs []int64) ([]stripPosting, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.responder(companyID, selectedIDs)
}

func TestStripBoilerplate_BelowThreshold_PassThrough(t *testing.T) {
	postings := []SelectedPosting{
		{PostingID: 1, CompanyID: 100, Title: "A", DescriptionText: "original A"},
		{PostingID: 2, CompanyID: 100, Title: "B", DescriptionText: "original B"},
	}

	runner := &fakeStripRunner{
		responder: func(companyID int64, selectedIDs []int64) ([]stripPosting, error) {
			t.Errorf("runner should not be invoked for company with < %d postings", stripMinPostings)
			return nil, nil
		},
	}

	got, err := StripBoilerplate(context.Background(), runner, postings)
	if err != nil {
		t.Fatalf("StripBoilerplate: %v", err)
	}
	if atomic.LoadInt32(&runner.calls) != 0 {
		t.Errorf("runner.calls: want 0, got %d", runner.calls)
	}
	for i, p := range got {
		if p.DescriptionText != postings[i].DescriptionText {
			t.Errorf("posting %d: want %q, got %q", p.PostingID, postings[i].DescriptionText, p.DescriptionText)
		}
	}
}

func TestStripBoilerplate_AtThreshold_UsesCleanedText(t *testing.T) {
	postings := []SelectedPosting{
		{PostingID: 1, CompanyID: 100, Title: "A", DescriptionText: "original A"},
		{PostingID: 2, CompanyID: 100, Title: "B", DescriptionText: "original B"},
		{PostingID: 3, CompanyID: 100, Title: "C", DescriptionText: "original C"},
	}

	runner := &fakeStripRunner{
		responder: func(companyID int64, selectedIDs []int64) ([]stripPosting, error) {
			return []stripPosting{
				{PostingID: 1, CleanedText: "cleaned A"},
				{PostingID: 2, CleanedText: "cleaned B"},
				{PostingID: 3, CleanedText: "cleaned C"},
			}, nil
		},
	}

	got, err := StripBoilerplate(context.Background(), runner, postings)
	if err != nil {
		t.Fatalf("StripBoilerplate: %v", err)
	}
	if atomic.LoadInt32(&runner.calls) != 1 {
		t.Errorf("runner.calls: want 1, got %d", runner.calls)
	}
	wants := map[int64]string{1: "cleaned A", 2: "cleaned B", 3: "cleaned C"}
	for _, p := range got {
		if p.DescriptionText != wants[p.PostingID] {
			t.Errorf("posting %d: want %q, got %q", p.PostingID, wants[p.PostingID], p.DescriptionText)
		}
	}
}

func TestStripBoilerplate_EmptyCleanedText_FallsBackToOriginal(t *testing.T) {
	postings := []SelectedPosting{
		{PostingID: 1, CompanyID: 100, Title: "A", DescriptionText: "original A"},
		{PostingID: 2, CompanyID: 100, Title: "B", DescriptionText: "original B"},
		{PostingID: 3, CompanyID: 100, Title: "C", DescriptionText: "original C"},
	}

	runner := &fakeStripRunner{
		responder: func(companyID int64, selectedIDs []int64) ([]stripPosting, error) {
			return []stripPosting{
				{PostingID: 1, CleanedText: "cleaned A"},
				{PostingID: 2, CleanedText: ""}, // empty → fallback
				// PostingID 3 missing entirely → fallback
			}, nil
		},
	}

	got, err := StripBoilerplate(context.Background(), runner, postings)
	if err != nil {
		t.Fatalf("StripBoilerplate: %v", err)
	}
	wants := map[int64]string{
		1: "cleaned A",
		2: "original B",
		3: "original C",
	}
	for _, p := range got {
		if p.DescriptionText != wants[p.PostingID] {
			t.Errorf("posting %d: want %q, got %q", p.PostingID, wants[p.PostingID], p.DescriptionText)
		}
	}
}

func TestStripBoilerplate_RunnerError_AbortsRun(t *testing.T) {
	postings := []SelectedPosting{
		{PostingID: 1, CompanyID: 100, DescriptionText: "x"},
		{PostingID: 2, CompanyID: 100, DescriptionText: "x"},
		{PostingID: 3, CompanyID: 100, DescriptionText: "x"},
	}

	sentinel := errors.New("strip failed")
	runner := &fakeStripRunner{
		responder: func(companyID int64, selectedIDs []int64) ([]stripPosting, error) {
			return nil, sentinel
		},
	}

	got, err := StripBoilerplate(context.Background(), runner, postings)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error chain: want sentinel %v, got %v", sentinel, err)
	}
	if got != nil {
		t.Errorf("expected nil result on error, got %v", got)
	}
}

func TestStripBoilerplate_MixedCompanies_OnlyAboveThresholdCallsRunner(t *testing.T) {
	postings := []SelectedPosting{
		// Company 100: 3 postings → above threshold, runner called.
		{PostingID: 1, CompanyID: 100, DescriptionText: "original 1"},
		{PostingID: 2, CompanyID: 100, DescriptionText: "original 2"},
		{PostingID: 3, CompanyID: 100, DescriptionText: "original 3"},
		// Company 200: 2 postings → below threshold, runner skipped.
		{PostingID: 4, CompanyID: 200, DescriptionText: "original 4"},
		{PostingID: 5, CompanyID: 200, DescriptionText: "original 5"},
	}

	var calledCompanies []int64
	runner := &fakeStripRunner{
		responder: func(companyID int64, selectedIDs []int64) ([]stripPosting, error) {
			calledCompanies = append(calledCompanies, companyID)
			return []stripPosting{
				{PostingID: 1, CleanedText: "cleaned 1"},
				{PostingID: 2, CleanedText: "cleaned 2"},
				{PostingID: 3, CleanedText: "cleaned 3"},
			}, nil
		},
	}

	got, err := StripBoilerplate(context.Background(), runner, postings)
	if err != nil {
		t.Fatalf("StripBoilerplate: %v", err)
	}
	if n := atomic.LoadInt32(&runner.calls); n != 1 {
		t.Errorf("runner.calls: want 1, got %d", n)
	}
	if len(calledCompanies) != 1 || calledCompanies[0] != 100 {
		t.Errorf("runner should have been called once for company 100; got %v", calledCompanies)
	}

	wants := map[int64]string{
		1: "cleaned 1",
		2: "cleaned 2",
		3: "cleaned 3",
		4: "original 4",
		5: "original 5",
	}
	for _, p := range got {
		if p.DescriptionText != wants[p.PostingID] {
			t.Errorf("posting %d: want %q, got %q", p.PostingID, wants[p.PostingID], p.DescriptionText)
		}
	}
}
