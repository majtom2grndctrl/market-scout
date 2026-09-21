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
		{"over max rejected", intPtr(501), false, false},
		{"min accepted", intPtr(1), true, true},
		{"max accepted", intPtr(500), true, true},
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
	env := runEnrichmentPreview(t.Context(), previewRequest{
		Count: intPtr(50), Focus: "go", Force: true, Sort: previewSortOldestFirst,
	}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	want := selection.Criteria{
		Count:         50,
		Focus:         "go",
		Force:         true,
		Sort:          selection.SortOldestFirst,
		Dedup:         true,
		MaxPerCompany: selection.DefaultMaxPerCompany,
	}
	if sel.gotCrit != want {
		t.Fatalf("selector got crit = %+v, want %+v", sel.gotCrit, want)
	}
	wantEcho := previewEcho{
		Count:         50,
		Focus:         "go",
		Force:         true,
		Sort:          previewSortOldestFirst,
		MaxPerCompany: selection.DefaultMaxPerCompany,
	}
	if env.Input != wantEcho {
		t.Fatalf("input echo = %+v, want %+v", env.Input, wantEcho)
	}
}

// The skill path and the Go binary must select the same cohort, so preview's
// default has to be the shared core's default, not its own. An omitted sort is
// newest-first because oldest-first drains the backlog in arrival order.
func TestRunEnrichmentPreview_SortDefaultsToNewestFirst(t *testing.T) {
	sel := &fakeSelector{}
	// Sort omitted entirely.
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10)}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.Input.Sort != previewSortNewestFirst {
		t.Fatalf("input.sort = %q, want default %q", env.Input.Sort, previewSortNewestFirst)
	}
	if sel.gotCrit.Sort != selection.SortNewestFirst {
		t.Fatalf("selector got sort = %v, want SortNewestFirst", sel.gotCrit.Sort)
	}
}

// An omitted max_per_company must reach the selector as the shared default
// rather than as Criteria's zero value, which the query reads as "no cap" —
// the one mistake that would silently restore single-company waves.
func TestRunEnrichmentPreview_MaxPerCompanyDefaultsToSharedDefault(t *testing.T) {
	sel := &fakeSelector{}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10)}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.Input.MaxPerCompany != selection.DefaultMaxPerCompany {
		t.Fatalf("input.max_per_company = %d, want %d", env.Input.MaxPerCompany, selection.DefaultMaxPerCompany)
	}
	if sel.gotCrit.MaxPerCompany != selection.DefaultMaxPerCompany {
		t.Fatalf("selector got max_per_company = %d, want %d", sel.gotCrit.MaxPerCompany, selection.DefaultMaxPerCompany)
	}
}

func TestRunEnrichmentPreview_MaxPerCompanyExplicitPassesThrough(t *testing.T) {
	sel := &fakeSelector{}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10), MaxPerCompany: intPtr(2)}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if sel.gotCrit.MaxPerCompany != 2 {
		t.Fatalf("selector got max_per_company = %d, want 2", sel.gotCrit.MaxPerCompany)
	}
	if env.Input.MaxPerCompany != 2 {
		t.Fatalf("input.max_per_company = %d, want 2", env.Input.MaxPerCompany)
	}
}

// Zero is out of range rather than "unset": Criteria's zero value means "no
// cap" inside the shared core, so a caller who passes 0 here must be rejected
// instead of quietly getting an uncapped wave.
func TestRunEnrichmentPreview_MaxPerCompanyOutOfRangeRejected(t *testing.T) {
	cases := []struct {
		name string
		give int
	}{
		{name: "zero", give: 0},
		{name: "negative", give: -1},
		{name: "above cap", give: previewMaxPerCompany + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sel := &fakeSelector{}
			env := runEnrichmentPreview(t.Context(), previewRequest{MaxPerCompany: intPtr(tc.give)}, sel)

			if env.Ok {
				t.Fatalf("env.Ok = true, want false for max_per_company %d", tc.give)
			}
			if sel.called {
				t.Fatalf("selector ran for an out-of-range max_per_company")
			}
			if len(env.Errors) != 1 || env.Errors[0].Code != codeInvalidMaxPerCompany || env.Errors[0].Path != "max_per_company" {
				t.Fatalf("errors = %+v, want one %s on max_per_company", env.Errors, codeInvalidMaxPerCompany)
			}
		})
	}
}

func TestRunEnrichmentPreview_SortExplicitNewestFirst(t *testing.T) {
	sel := &fakeSelector{}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10), Sort: "newest_first"}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.Input.Sort != previewSortNewestFirst {
		t.Fatalf("input.sort = %q, want %q", env.Input.Sort, previewSortNewestFirst)
	}
	if sel.gotCrit.Sort != selection.SortNewestFirst {
		t.Fatalf("selector got sort = %v, want SortNewestFirst", sel.gotCrit.Sort)
	}
}

func TestRunEnrichmentPreview_SortExplicitOldestFirst(t *testing.T) {
	sel := &fakeSelector{}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10), Sort: "oldest_first"}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.Input.Sort != previewSortOldestFirst {
		t.Fatalf("input.sort = %q, want %q", env.Input.Sort, previewSortOldestFirst)
	}
	if sel.gotCrit.Sort != selection.SortOldestFirst {
		t.Fatalf("selector got sort = %v, want SortOldestFirst", sel.gotCrit.Sort)
	}
}

func TestRunEnrichmentPreview_SortInvalidValueRejected(t *testing.T) {
	sel := &fakeSelector{}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10), Sort: "latest"}, sel)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false for invalid sort value")
	}
	if sel.called {
		t.Fatalf("selector called = true, want false when sort is invalid")
	}
	if !hasError(env.Errors, "sort", codeInvalidSort) {
		t.Fatalf("errors = %+v, want path=sort code=%s", env.Errors, codeInvalidSort)
	}
	if env.Input.Sort != "latest" {
		t.Fatalf("input.sort = %q, want echo of raw invalid value %q", env.Input.Sort, "latest")
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

	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(500), Force: true}, sel)

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
	if len(env.Postings) != len(postings) {
		t.Fatalf("len(postings) = %d, want complete selected list of %d", len(env.Postings), len(postings))
	}
	last := env.Postings[len(env.Postings)-1]
	if last.PostingID != 25 || last.CompanyID != 124 || last.CompanyName != "Acme" || last.Title != "Engineer" {
		t.Fatalf("postings[last] = %+v, want final ordered selected posting", last)
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
	// sample and postings must marshal as [] not null so the agent sees consistent shapes.
	var decoded struct {
		Sample   json.RawMessage `json:"sample"`
		Postings json.RawMessage `json:"postings"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if string(decoded.Sample) != "[]" {
		t.Fatalf("sample = %s, want []", decoded.Sample)
	}
	if string(decoded.Postings) != "[]" {
		t.Fatalf("postings = %s, want []", decoded.Postings)
	}
}

// Preview always dedups: the direct-MCP coordinator dispatches exactly the list
// this tool returns and is not allowed to reimplement selection SQL, so a
// preview that forwarded Dedup=false would hand it ungrouped postings.
func TestRunEnrichmentPreview_AlwaysRequestsDedup(t *testing.T) {
	for _, force := range []bool{false, true} {
		sel := &fakeSelector{}
		runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(10), Force: force}, sel)
		if !sel.gotCrit.Dedup {
			t.Fatalf("force=%v: selector got Dedup=false, want true", force)
		}
	}
}

// A work unit's siblings ride along past the count limit, so selected_count
// counts postings while work_unit_count counts the bounded thing. Reporting only
// one of them is how "25 postings" silently became "25 jobs, 31 postings".
func TestRunEnrichmentPreview_ReportsWorkUnitsAndPostingsSeparately(t *testing.T) {
	sel := &fakeSelector{postings: []selection.Posting{
		{PostingID: 1, DedupKey: "7|unit:1", IsRepresentative: true},
		{PostingID: 2, DedupKey: "7|unit:1"},
		{PostingID: 3, DedupKey: "7|unit:1"},
		{PostingID: 4, DedupKey: "7|unit:4", IsRepresentative: true},
	}}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(2)}, sel)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.WorkUnitCount != 2 {
		t.Fatalf("work_unit_count = %d, want 2", env.WorkUnitCount)
	}
	if env.SelectedCount != 4 {
		t.Fatalf("selected_count = %d, want 4 (siblings ride along past count)", env.SelectedCount)
	}
}

// The coordinator needs the grouping on every row it dispatches, not just in
// the capped sample, or it cannot tell a representative from a sibling.
func TestRunEnrichmentPreview_PostingsCarryDedupFields(t *testing.T) {
	sel := &fakeSelector{postings: []selection.Posting{
		{PostingID: 1, DedupKey: "7|unit:1", IsRepresentative: true},
		{PostingID: 2, DedupKey: "7|unit:1"},
	}}
	env := runEnrichmentPreview(t.Context(), previewRequest{Count: intPtr(5)}, sel)

	if len(env.Postings) != 2 {
		t.Fatalf("len(postings) = %d, want 2", len(env.Postings))
	}
	if env.Postings[0].DedupKey != "7|unit:1" || !env.Postings[0].IsRepresentative {
		t.Fatalf("postings[0] = %+v, want dedup_key 7|unit:1 and representative", env.Postings[0])
	}
	if env.Postings[1].IsRepresentative {
		t.Fatalf("postings[1] = %+v, want sibling (is_representative false)", env.Postings[1])
	}
}
