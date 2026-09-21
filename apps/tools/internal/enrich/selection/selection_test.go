package selection

import (
	"context"
	"database/sql"
	"errors"
	"math"
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
	gotUnclassifiedArg db.ListUnclassifiedPostingsParams
	gotForcedArg       db.ListUnclassifiedPostingsForcedParams
}

func (f *fakeQuerier) ListUnclassifiedPostings(ctx context.Context, arg db.ListUnclassifiedPostingsParams) ([]db.ListUnclassifiedPostingsRow, error) {
	f.calledUnclassified = true
	f.gotUnclassifiedArg = arg
	if f.unclassifiedErr != nil {
		return nil, f.unclassifiedErr
	}
	return f.unclassified, nil
}

func (f *fakeQuerier) ListUnclassifiedPostingsForced(ctx context.Context, arg db.ListUnclassifiedPostingsForcedParams) ([]db.ListUnclassifiedPostingsForcedRow, error) {
	f.calledForced = true
	f.gotForcedArg = arg
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

// The zero value carries the package's policy, so a caller that never mentions
// Sort still gets recency-first. Draining oldest-first is what kept the
// classified set months behind the market; a caller has to ask for it now.
func TestSelectRows_SortZeroValueMapsToNewestFirst(t *testing.T) {
	q := &fakeQuerier{}
	if _, err := selectRows(t.Context(), q, Criteria{Count: 10}); err != nil {
		t.Fatalf("selectRows: %v", err)
	}
	if !q.gotUnclassifiedArg.NewestFirst {
		t.Fatalf("NewestFirst = false, want true for zero-value Sort")
	}
}

func TestSelectRows_SortOldestFirstIsStillReachable(t *testing.T) {
	q := &fakeQuerier{}
	if _, err := selectRows(t.Context(), q, Criteria{Count: 10, Sort: SortOldestFirst}); err != nil {
		t.Fatalf("selectRows: %v", err)
	}
	if q.gotUnclassifiedArg.NewestFirst {
		t.Fatalf("NewestFirst = true, want false for SortOldestFirst")
	}
}

// The cap's whole purpose is that nobody has to remember to ask for it, so the
// zero value has to resolve to the default rather than to "no cap". Both query
// variants get it: a --force re-enrichment run is exactly when one company's
// backlog would otherwise swallow the wave.
func TestSelectRows_MaxPerCompanyZeroTakesDefault(t *testing.T) {
	t.Run("unclassified", func(t *testing.T) {
		q := &fakeQuerier{}
		if _, err := selectRows(t.Context(), q, Criteria{Count: 10}); err != nil {
			t.Fatalf("selectRows: %v", err)
		}
		if q.gotUnclassifiedArg.MaxPerCompany != DefaultMaxPerCompany {
			t.Fatalf("MaxPerCompany = %d, want DefaultMaxPerCompany (%d)", q.gotUnclassifiedArg.MaxPerCompany, DefaultMaxPerCompany)
		}
	})

	t.Run("forced", func(t *testing.T) {
		q := &fakeQuerier{}
		if _, err := selectRows(t.Context(), q, Criteria{Count: 10, Force: true}); err != nil {
			t.Fatalf("selectRows: %v", err)
		}
		if q.gotForcedArg.MaxPerCompany != DefaultMaxPerCompany {
			t.Fatalf("MaxPerCompany = %d, want DefaultMaxPerCompany (%d)", q.gotForcedArg.MaxPerCompany, DefaultMaxPerCompany)
		}
	})
}

func TestSelectRows_MaxPerCompanyExplicitAndDisabled(t *testing.T) {
	cases := []struct {
		name string
		give int
		want int32
	}{
		{name: "explicit value passes through", give: 3, want: 3},
		{name: "NoCompanyCap disables the cap", give: NoCompanyCap, want: 0},
		{name: "any negative disables the cap", give: -17, want: 0},
		{name: "oversized value clamps instead of wrapping", give: math.MaxInt32 + 1, want: math.MaxInt32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQuerier{}
			if _, err := selectRows(t.Context(), q, Criteria{Count: 10, MaxPerCompany: tc.give}); err != nil {
				t.Fatalf("selectRows: %v", err)
			}
			if q.gotUnclassifiedArg.MaxPerCompany != tc.want {
				t.Fatalf("MaxPerCompany = %d, want %d", q.gotUnclassifiedArg.MaxPerCompany, tc.want)
			}
		})
	}
}

func TestSelectRows_SortNewestFirstSetsParam(t *testing.T) {
	q := &fakeQuerier{}
	if _, err := selectRows(t.Context(), q, Criteria{Count: 10, Sort: SortNewestFirst}); err != nil {
		t.Fatalf("selectRows: %v", err)
	}
	if !q.gotUnclassifiedArg.NewestFirst {
		t.Fatalf("NewestFirst = false, want true for SortNewestFirst")
	}
}

func TestSelectRows_SortNewestFirstSetsParamOnForcedVariant(t *testing.T) {
	q := &fakeQuerier{}
	if _, err := selectRows(t.Context(), q, Criteria{Count: 10, Force: true, Sort: SortNewestFirst}); err != nil {
		t.Fatalf("selectRows: %v", err)
	}
	if !q.gotForcedArg.NewestFirst {
		t.Fatalf("NewestFirst = false, want true for SortNewestFirst on forced variant")
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

// Dedup has to reach both query variants. Forgetting the forced path would make
// --force runs silently ungrouped — the one mode where duplicate postings are
// guaranteed to already exist.
func TestSelectWith_ForwardsDedupToBothVariants(t *testing.T) {
	t.Run("unclassified", func(t *testing.T) {
		q := &fakeQuerier{}
		if _, _, err := SelectWith(t.Context(), q, Criteria{Count: 5, Dedup: true}); err != nil {
			t.Fatalf("SelectWith: %v", err)
		}
		if !q.gotUnclassifiedArg.Dedup {
			t.Fatalf("Dedup = false in params, want true")
		}
	})

	t.Run("forced", func(t *testing.T) {
		q := &fakeQuerier{}
		if _, _, err := SelectWith(t.Context(), q, Criteria{Count: 5, Force: true, Dedup: true}); err != nil {
			t.Fatalf("SelectWith: %v", err)
		}
		if !q.gotForcedArg.Dedup {
			t.Fatalf("Dedup = false in forced params, want true")
		}
	})
}

// The grouping columns are the whole point of the query change; dropping them
// in the row mapping would leave callers unable to tell a representative from a
// sibling while everything still compiled.
func TestSelectWith_CarriesDedupColumnsThrough(t *testing.T) {
	q := &fakeQuerier{unclassified: []db.ListUnclassifiedPostingsRow{
		{PostingID: 11, CompanyID: 3, CompanyName: "Acme", DedupKey: "3|unit:11", IsRepresentative: true},
		{PostingID: 12, CompanyID: 3, CompanyName: "Acme", DedupKey: "3|unit:11", IsRepresentative: false},
	}}

	got, _, err := SelectWith(t.Context(), q, Criteria{Count: 1, Dedup: true})
	if err != nil {
		t.Fatalf("SelectWith: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(postings) = %d, want 2", len(got))
	}
	if got[0].DedupKey != "3|unit:11" || !got[0].IsRepresentative {
		t.Errorf("postings[0] = %+v, want dedup key 3|unit:11 and representative", got[0])
	}
	if got[1].DedupKey != got[0].DedupKey || got[1].IsRepresentative {
		t.Errorf("postings[1] = %+v, want same key as its representative and is_representative=false", got[1])
	}
}
