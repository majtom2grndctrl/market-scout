package selection

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// fakeQuerier stands in for *db.Queries so selection logic can be exercised
// without a database. It records which variant ran and returns canned rows.
type fakeQuerier struct {
	unclassified       []db.ListUnclassifiedPostingsRow
	forced             []db.ListUnclassifiedPostingsForcedRow
	classified         []int64
	unclassifiedErr    error
	forcedErr          error
	classifiedErr      error
	calledUnclassified bool
	calledForced       bool
	classifiedAmongIDs []int64
}

func (f *fakeQuerier) ListUnclassifiedPostings(ctx context.Context, arg db.ListUnclassifiedPostingsParams) ([]db.ListUnclassifiedPostingsRow, error) {
	f.calledUnclassified = true
	if f.unclassifiedErr != nil {
		return nil, f.unclassifiedErr
	}
	return f.unclassified, nil
}

func (f *fakeQuerier) ListUnclassifiedPostingsForced(ctx context.Context, arg db.ListUnclassifiedPostingsForcedParams) ([]db.ListUnclassifiedPostingsForcedRow, error) {
	f.calledForced = true
	if f.forcedErr != nil {
		return nil, f.forcedErr
	}
	return f.forced, nil
}

func (f *fakeQuerier) ListClassifiedAmong(ctx context.Context, ids []int64) ([]int64, error) {
	f.classifiedAmongIDs = ids
	if f.classifiedErr != nil {
		return nil, f.classifiedErr
	}
	return f.classified, nil
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func TestSelectWith_UnclassifiedVariant(t *testing.T) {
	q := &fakeQuerier{
		unclassified: []db.ListUnclassifiedPostingsRow{
			{PostingID: 1, CompanyID: 10, CompanyName: "Acme", Title: nullString("Engineer"), DescriptionText: nullString("desc")},
			{PostingID: 2, CompanyID: 11, CompanyName: "Beta", Title: sql.NullString{}, DescriptionText: nullString("desc2")},
		},
	}

	postings, already, err := SelectWith(t.Context(), q, Criteria{Count: 10, Focus: "go", Force: false})
	if err != nil {
		t.Fatalf("SelectWith: %v", err)
	}
	if q.calledForced {
		t.Fatalf("forced variant ran for force=false")
	}
	if !q.calledUnclassified {
		t.Fatalf("unclassified variant did not run for force=false")
	}
	if already != nil {
		t.Fatalf("alreadyClassified = %v, want nil for force=false", already)
	}
	if len(postings) != 2 {
		t.Fatalf("len(postings) = %d, want 2", len(postings))
	}
	if postings[0].CompanyName != "Acme" || postings[0].Title != "Engineer" {
		t.Fatalf("postings[0] = %+v, want company Acme title Engineer", postings[0])
	}
	// Null title falls back to empty string.
	if postings[1].Title != "" {
		t.Fatalf("postings[1].Title = %q, want empty for null title", postings[1].Title)
	}
	if postings[1].CompanyName != "Beta" {
		t.Fatalf("postings[1].CompanyName = %q, want Beta", postings[1].CompanyName)
	}
}

func TestSelectWith_ForcedVariantCountsClassified(t *testing.T) {
	q := &fakeQuerier{
		forced: []db.ListUnclassifiedPostingsForcedRow{
			{PostingID: 1, CompanyID: 10, CompanyName: "Acme", Title: nullString("Engineer"), DescriptionText: nullString("desc")},
			{PostingID: 2, CompanyID: 11, CompanyName: "Beta", Title: nullString("Designer"), DescriptionText: nullString("desc2")},
		},
		classified: []int64{2},
	}

	postings, already, err := SelectWith(t.Context(), q, Criteria{Count: 10, Force: true})
	if err != nil {
		t.Fatalf("SelectWith: %v", err)
	}
	if !q.calledForced {
		t.Fatalf("forced variant did not run for force=true")
	}
	if q.calledUnclassified {
		t.Fatalf("unclassified variant ran for force=true")
	}
	if len(postings) != 2 {
		t.Fatalf("len(postings) = %d, want 2", len(postings))
	}
	// The forced path passes selected posting IDs to ListClassifiedAmong.
	if len(q.classifiedAmongIDs) != 2 || q.classifiedAmongIDs[0] != 1 || q.classifiedAmongIDs[1] != 2 {
		t.Fatalf("classifiedAmongIDs = %v, want [1 2]", q.classifiedAmongIDs)
	}
	if len(already) != 1 || already[0] != 2 {
		t.Fatalf("alreadyClassified = %v, want [2]", already)
	}
}

func TestSelectWith_ForcedEmptySkipsClassifiedCheck(t *testing.T) {
	q := &fakeQuerier{forced: nil}

	postings, already, err := SelectWith(t.Context(), q, Criteria{Count: 10, Force: true})
	if err != nil {
		t.Fatalf("SelectWith: %v", err)
	}
	if len(postings) != 0 {
		t.Fatalf("len(postings) = %d, want 0", len(postings))
	}
	if already != nil {
		t.Fatalf("alreadyClassified = %v, want nil when no postings selected", already)
	}
	if q.classifiedAmongIDs != nil {
		t.Fatalf("ListClassifiedAmong called with %v, want skipped for empty selection", q.classifiedAmongIDs)
	}
}

func TestSelectWith_QueryErrorWraps(t *testing.T) {
	q := &fakeQuerier{unclassifiedErr: errors.New("boom")}
	_, _, err := SelectWith(t.Context(), q, Criteria{Count: 10})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, q.unclassifiedErr) {
		t.Fatalf("error %v does not wrap underlying query error", err)
	}
}
