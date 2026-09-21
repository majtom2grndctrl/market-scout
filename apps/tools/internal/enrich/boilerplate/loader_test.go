package boilerplate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeLoader struct {
	corpus    []CorpusPosting
	companies map[int64]int64
	err       error
}

func (f fakeLoader) LoadCompanyCorpus(context.Context, int64) ([]CorpusPosting, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.corpus, nil
}

func (f fakeLoader) FindPostingCompanies(context.Context, []int64) (map[int64]int64, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.companies, nil
}

func TestCleanSelected_UsesUnselectedCompanySiblingAsBoilerplateEvidence(t *testing.T) {
	shared := strings.Repeat("Shared company background. ", 10)
	loader := fakeLoader{
		corpus: []CorpusPosting{
			{PostingID: 11, DescriptionText: shared + "Backend role requirements."},
			{PostingID: 12, DescriptionText: "Independent short description."},
			{PostingID: 13, DescriptionText: shared + "Unselected sibling role requirements."},
		},
		companies: map[int64]int64{11: 7, 12: 7, 13: 7},
	}

	got, err := CleanSelected(t.Context(), loader, 7, []int64{11})
	if err != nil {
		t.Fatalf("CleanSelected: %v", err)
	}
	if len(got) != 1 || got[0].PostingID != 11 {
		t.Fatalf("result = %+v, want ordered selected posting 11", got)
	}
	if got[0].DescriptionText != "Backend role requirements." {
		t.Fatalf("cleaned text = %q, want unselected sibling evidence to strip shared text", got[0].DescriptionText)
	}
}

func TestCleanSelected_ReportsDuplicateUnknownAndCrossCompanyIDs(t *testing.T) {
	loader := fakeLoader{
		corpus:    []CorpusPosting{{PostingID: 11, DescriptionText: "description"}},
		companies: map[int64]int64{11: 7, 22: 8},
	}

	_, err := CleanSelected(t.Context(), loader, 7, []int64{11, 11, 99, 22})
	var invalid *InvalidSelectionError
	if !errors.As(err, &invalid) {
		t.Fatalf("duplicate error = %v, want InvalidSelectionError", err)
	}
	if len(invalid.Issues) != 3 || invalid.Issues[0].Index != 1 || invalid.Issues[0].Code != CodeDuplicateSelectedID || invalid.Issues[1].Code != CodeUnknownPostingID || invalid.Issues[2].Code != CodeCrossCompanyPostingID {
		t.Fatalf("mixed selection issues = %+v", invalid.Issues)
	}
}
