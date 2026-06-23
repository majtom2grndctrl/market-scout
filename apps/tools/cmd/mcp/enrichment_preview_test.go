package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

// fakeSelector returns canned postings (or an error) and records the criteria it
// was called with, so tests can assert default application and response mapping
// without a database.
type fakeSelector struct {
	postings   []selection.Posting
	classified []int64
	err        error
	called     bool
	gotCrit    selection.Criteria
}

func (f *fakeSelector) Select(ctx context.Context, crit selection.Criteria) ([]selection.Posting, []int64, error) {
	f.called = true
	f.gotCrit = crit
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.postings, f.classified, nil
}

func intPtr(i int) *int { return &i }

func TestRunEnrichmentPreview_CountBoundValidation(t *testing.T) {
	tests := []struct {
		name      string
		count     *int
		wantOk    bool
		wantCalls bool
	}{
		{"zero rejected", intPtr(0), false, false},
		{"over max rejected", intPtr(101), false, false},
		{"min accepted", intPtr(1), true, true},
		{"max accepted", intPtr(100), true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sel := &fakeSelector{}
			env := runEnrichmentPreview(t.Context(), previewRequest{Count: tc.count}, sel)

			if env.Ok != tc.wantOk {
				t.Fatalf("env.Ok = %v, want %v; errors=%+v", env.Ok, tc.wantOk, env.Errors)
			}
			if sel.called != tc.wantCalls {
				t.Fatalf("selector called = %v, want %v", sel.called, tc.wantCalls)
			}
			if !tc.wantOk {
				if !hasError(env.Errors, "count", codeInvalidCount) {
					t.Fatalf("errors = %+v, want path=count code=%s", env.Errors, codeInvalidCount)
				}
				if env.Input.Count != *tc.count {
					t.Fatalf("input.count = %d, want echo of %d", env.Input.Count, *tc.count)
				}
			}
		})
	}
}

func TestRunEnrichmentPreview_DefaultsApplied(t *testing.T) {
	sel := &fakeSelector{}
	// Count omitted; focus/force default to zero values.
	env := runEnrichmentPreview(t.Context(), previewRequest{}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.Input.Count != previewDefaultCount {
		t.Fatalf("input.count = %d, want default %d", env.Input.Count, previewDefaultCount)
	}
	if sel.gotCrit.Count != previewDefaultCount {
		t.Fatalf("selector got count = %d, want default %d", sel.gotCrit.Count, previewDefaultCount)
	}
	if sel.gotCrit.Focus != "" || sel.gotCrit.Force {
		t.Fatalf("selector got crit = %+v, want empty focus and force=false", sel.gotCrit)
	}
}

func TestRunEnrichmentPreview_PassesInputsToSelector(t *testing.T) {
	sel := &fakeSelector{}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(50), Focus: "go", Force: true}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	want := selection.Criteria{Count: 50, Focus: "go", Force: true}
	if sel.gotCrit != want {
		t.Fatalf("selector got crit = %+v, want %+v", sel.gotCrit, want)
	}
	if env.Input != (previewEcho{Count: 50, Focus: "go", Force: true}) {
		t.Fatalf("input echo = %+v, want {50 go true}", env.Input)
	}
}

func TestRunEnrichmentPreview_ResponseMapping(t *testing.T) {
	postings := make([]selection.Posting, 0, 25)
	for i := 0; i < 25; i++ {
		postings = append(postings, selection.Posting{
			PostingID:   int64(i + 1),
			CompanyID:   int64(100 + i),
			CompanyName: "Acme",
			Title:       "Engineer",
		})
	}
	sel := &fakeSelector{postings: postings, classified: []int64{1, 2, 3}}

	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(100), Force: true}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	// selected_count is the full count-limited selection, not the sample size.
	if env.SelectedCount != 25 {
		t.Fatalf("selected_count = %d, want 25", env.SelectedCount)
	}
	// sample is capped at 20.
	if env.SampleCount != previewSampleCap {
		t.Fatalf("sample_count = %d, want %d", env.SampleCount, previewSampleCap)
	}
	if len(env.Sample) != previewSampleCap {
		t.Fatalf("len(sample) = %d, want %d", len(env.Sample), previewSampleCap)
	}
	if env.AlreadyClassifiedCount != 3 {
		t.Fatalf("already_classified_count = %d, want 3", env.AlreadyClassifiedCount)
	}
	first := env.Sample[0]
	if first.PostingID != 1 || first.CompanyID != 100 || first.CompanyName != "Acme" || first.Title != "Engineer" {
		t.Fatalf("sample[0] = %+v, want posting 1 / company 100 Acme / Engineer", first)
	}
}

func TestRunEnrichmentPreview_AlreadyClassifiedZeroWhenForceFalse(t *testing.T) {
	// force=false: shared selection returns nil for alreadyClassified, so the
	// count must be 0 and always present.
	sel := &fakeSelector{
		postings:   []selection.Posting{{PostingID: 1, CompanyID: 10, CompanyName: "Acme", Title: "Eng"}},
		classified: nil,
	}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10), Force: false}, sel)

	if env.AlreadyClassifiedCount != 0 {
		t.Fatalf("already_classified_count = %d, want 0 for force=false", env.AlreadyClassifiedCount)
	}
	// Confirm the field is present in JSON even at zero.
	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := decoded["already_classified_count"]; !ok {
		t.Fatalf("already_classified_count absent from JSON; want always present")
	}
}

func TestRunEnrichmentPreview_SelectorErrorReturnsEnvelope(t *testing.T) {
	sel := &fakeSelector{err: errors.New("conn reset")}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10)}, sel)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false on selector error")
	}
	if !hasError(env.Errors, "db", codeDBError) {
		t.Fatalf("errors = %+v, want path=db code=%s", env.Errors, codeDBError)
	}
}

func TestRunEnrichmentPreview_EmptySelectionEncodesEmptySample(t *testing.T) {
	sel := &fakeSelector{postings: nil}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10)}, sel)

	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	// sample must marshal as [] not null so the agent sees a consistent shape.
	var decoded struct {
		Sample json.RawMessage `json:"sample"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if string(decoded.Sample) != "[]" {
		t.Fatalf("sample = %s, want []", decoded.Sample)
	}
}
