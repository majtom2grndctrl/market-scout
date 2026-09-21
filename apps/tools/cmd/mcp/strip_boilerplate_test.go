package main

import (
	"context"
	"strings"
	"testing"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/boilerplate"
)

type fakeBoilerplateLoader struct {
	corpus    []boilerplate.CorpusPosting
	companies map[int64]int64
}

func (f fakeBoilerplateLoader) LoadCompanyCorpus(context.Context, int64) ([]boilerplate.CorpusPosting, error) {
	return f.corpus, nil
}

func (f fakeBoilerplateLoader) FindPostingCompanies(context.Context, []int64) (map[int64]int64, error) {
	return f.companies, nil
}

func TestRunStripBoilerplate_ReturnsOnlyOrderedSelectedPostings(t *testing.T) {
	shared := strings.Repeat("Shared company background. ", 10)
	loader := fakeBoilerplateLoader{
		corpus: []boilerplate.CorpusPosting{
			{PostingID: 11, DescriptionText: shared + "First selected."},
			{PostingID: 12, DescriptionText: "Independent description."},
			{PostingID: 13, DescriptionText: shared + "Unselected sibling."},
		},
		companies: map[int64]int64{11: 7, 12: 7, 13: 7},
	}

	env := runStripBoilerplate(t.Context(), stripBoilerplateRequest{CompanyID: 7, SelectedIDs: []int64{11}}, loader)
	if !env.Ok || len(env.Postings) != 1 {
		t.Fatalf("envelope = %+v, want one successful selected result", env)
	}
	if env.Postings[0].PostingID != 11 || env.Postings[0].CleanedText != "First selected." {
		t.Fatalf("posting = %+v, want cleaned selected posting", env.Postings[0])
	}
}

func TestRunStripBoilerplate_ReturnsStructuredInvalidIDErrors(t *testing.T) {
	loader := fakeBoilerplateLoader{
		corpus:    []boilerplate.CorpusPosting{{PostingID: 11, DescriptionText: "description"}},
		companies: map[int64]int64{11: 7, 22: 8},
	}

	env := runStripBoilerplate(t.Context(), stripBoilerplateRequest{CompanyID: 7, SelectedIDs: []int64{11, 11}}, loader)
	if env.Ok || !hasError(env.Errors, "selected_ids[1]", string(boilerplate.CodeDuplicateSelectedID)) {
		t.Fatalf("duplicate envelope = %+v, want structured duplicate error", env)
	}

	env = runStripBoilerplate(t.Context(), stripBoilerplateRequest{CompanyID: 7, SelectedIDs: []int64{99, 22}}, loader)
	if env.Ok || !hasError(env.Errors, "selected_ids[0]", string(boilerplate.CodeUnknownPostingID)) || !hasError(env.Errors, "selected_ids[1]", string(boilerplate.CodeCrossCompanyPostingID)) {
		t.Fatalf("ownership envelope = %+v, want structured unknown and cross-company errors", env)
	}
}
