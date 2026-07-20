package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type fakeDedupTokenCall struct {
	ATS         string
	BoardToken  string
	RecencyDays int32
}

type fakeDedupNameCall struct {
	Names       []string
	RecencyDays int32
}

type fakeDedupSimilarityCall struct {
	Names       []string
	RecencyDays int32
}

type fakeDedupDomainCall struct {
	CareersURLs []string
	RecencyDays int32
}

type fakeDedupUnsupportedNameCall struct {
	Names []string
}

type fakeDedupUnsupportedDomainCall struct {
	URLs []string
}

type fakeDedupSource struct {
	tokenMatches             map[string]dedupMatchedCompany
	nameMatches              map[int][]dedupMatchedCompany
	similarityMatches        map[int][]dedupMatchedCompany
	domainMatches            map[int][]dedupMatchedCompany
	unsupportedNameMatches   map[int]dedupUnsupportedRecord
	unsupportedDomainMatches map[int]dedupUnsupportedRecord
	tokenErr                 error
	nameErr                  error
	similarityErr            error
	domainErr                error
	unsupportedNameErr       error
	unsupportedDomainErr     error
	tokenCalls               []fakeDedupTokenCall
	nameCalls                []fakeDedupNameCall
	similarityCalls          []fakeDedupSimilarityCall
	domainCalls              []fakeDedupDomainCall
	unsupportedNameCalls     []fakeDedupUnsupportedNameCall
	unsupportedDomainCalls   []fakeDedupUnsupportedDomainCall
}

func (f *fakeDedupSource) FindUnsupportedByNames(ctx context.Context, names []string) (map[int]dedupUnsupportedRecord, error) {
	f.unsupportedNameCalls = append(f.unsupportedNameCalls, fakeDedupUnsupportedNameCall{Names: append([]string(nil), names...)})
	if f.unsupportedNameErr != nil {
		return nil, f.unsupportedNameErr
	}
	if f.unsupportedNameMatches == nil {
		return map[int]dedupUnsupportedRecord{}, nil
	}
	return f.unsupportedNameMatches, nil
}

func (f *fakeDedupSource) FindUnsupportedByURLHost(ctx context.Context, urls []string) (map[int]dedupUnsupportedRecord, error) {
	f.unsupportedDomainCalls = append(f.unsupportedDomainCalls, fakeDedupUnsupportedDomainCall{URLs: append([]string(nil), urls...)})
	if f.unsupportedDomainErr != nil {
		return nil, f.unsupportedDomainErr
	}
	if f.unsupportedDomainMatches == nil {
		return map[int]dedupUnsupportedRecord{}, nil
	}
	return f.unsupportedDomainMatches, nil
}

func (f *fakeDedupSource) FindByCareersURLHost(ctx context.Context, careersURLs []string, recencyDays int32) (map[int][]dedupMatchedCompany, error) {
	f.domainCalls = append(f.domainCalls, fakeDedupDomainCall{CareersURLs: append([]string(nil), careersURLs...), RecencyDays: recencyDays})
	if f.domainErr != nil {
		return nil, f.domainErr
	}
	return f.domainMatches, nil
}

func (f *fakeDedupSource) FindByNameSimilarity(ctx context.Context, names []string, recencyDays int32) (map[int][]dedupMatchedCompany, error) {
	f.similarityCalls = append(f.similarityCalls, fakeDedupSimilarityCall{Names: append([]string(nil), names...), RecencyDays: recencyDays})
	if f.similarityErr != nil {
		return nil, f.similarityErr
	}
	return f.similarityMatches, nil
}

func (f *fakeDedupSource) FindByToken(ctx context.Context, ats, boardToken string, recencyDays int32) (*dedupMatchedCompany, error) {
	f.tokenCalls = append(f.tokenCalls, fakeDedupTokenCall{ATS: ats, BoardToken: boardToken, RecencyDays: recencyDays})
	if f.tokenErr != nil {
		return nil, f.tokenErr
	}
	match, ok := f.tokenMatches[dedupTokenKey(ats, boardToken)]
	if !ok {
		return nil, nil
	}
	return &match, nil
}

func (f *fakeDedupSource) FindByNames(ctx context.Context, names []string, recencyDays int32) (map[int][]dedupMatchedCompany, error) {
	f.nameCalls = append(f.nameCalls, fakeDedupNameCall{Names: append([]string(nil), names...), RecencyDays: recencyDays})
	if f.nameErr != nil {
		return nil, f.nameErr
	}
	return f.nameMatches, nil
}

func dedupTokenKey(ats, boardToken string) string {
	return ats + "\x00" + boardToken
}

func TestRunDedupCandidates_TokenMatchMapsRecencyToVerdict(t *testing.T) {
	tests := []struct {
		name              string
		hasRecentSnapshot bool
		wantVerdict       string
		wantReason        string
	}{
		{"recent snapshot duplicate", true, dedupVerdictDuplicate, dedupReasonMatchedByTokenRecent},
		{"no recent snapshot stale", false, dedupVerdictStale, dedupReasonMatchedByTokenStale},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			match := dedupMatchedCompany{
				ID:                42,
				Name:              "Acme",
				ATS:               "greenhouse",
				BoardToken:        "acme",
				HasRecentSnapshot: tc.hasRecentSnapshot,
				MatchKind:         dedupMatchKindToken,
			}
			source := &fakeDedupSource{
				tokenMatches: map[string]dedupMatchedCompany{
					dedupTokenKey("greenhouse", "acme"): match,
				},
			}

			env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
				Candidates: []dedupCandidateInput{{
					Name:       "Acme",
					ATS:        "greenhouse",
					BoardToken: "acme",
				}},
			}, source)

			if !env.Ok {
				t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
			}
			if len(env.Results) != 1 {
				t.Fatalf("len(results) = %d, want 1", len(env.Results))
			}
			got := env.Results[0]
			if got.Verdict != tc.wantVerdict {
				t.Fatalf("verdict = %q, want %q", got.Verdict, tc.wantVerdict)
			}
			if got.MatchKind != dedupMatchKindToken {
				t.Fatalf("match_kind = %q, want %q", got.MatchKind, dedupMatchKindToken)
			}
			if got.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if got.Matched == nil || *got.Matched != match {
				t.Fatalf("matched = %+v, want %+v", got.Matched, match)
			}
			if got.MatchCount != 1 {
				t.Fatalf("match_count = %d, want 1", got.MatchCount)
			}
			if !reflect.DeepEqual(got.Matches, []dedupMatchedCompany{match}) {
				t.Fatalf("matches = %+v, want primary token match", got.Matches)
			}
			if len(source.nameCalls) != 0 {
				t.Fatalf("name lookup called after token match: %+v", source.nameCalls)
			}
		})
	}
}

func TestRunDedupCandidates_NameMatchMapsToStaleWithAllMatchedCompanies(t *testing.T) {
	primary := dedupMatchedCompany{
		ID:                77,
		Name:              "Acme Inc",
		ATS:               "ashby",
		BoardToken:        "acme-inc",
		HasRecentSnapshot: true,
		MatchKind:         dedupMatchKindNameOnly,
	}
	secondary := dedupMatchedCompany{
		ID:                93,
		Name:              "A.C.M.E. Inc",
		ATS:               "greenhouse",
		BoardToken:        "acme",
		HasRecentSnapshot: false,
		MatchKind:         dedupMatchKindNameOnly,
	}
	source := &fakeDedupSource{
		nameMatches: map[int][]dedupMatchedCompany{
			0: {primary, secondary},
		},
	}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
		Candidates: []dedupCandidateInput{{Name: "  Acme, Inc.  "}},
	}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if len(env.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(env.Results))
	}
	got := env.Results[0]
	if got.Verdict != dedupVerdictStale {
		t.Fatalf("verdict = %q, want %q", got.Verdict, dedupVerdictStale)
	}
	if got.MatchKind != dedupMatchKindNameOnly {
		t.Fatalf("match_kind = %q, want %q", got.MatchKind, dedupMatchKindNameOnly)
	}
	if got.Reason != dedupReasonMatchedByNameOnly {
		t.Fatalf("reason = %q, want %q", got.Reason, dedupReasonMatchedByNameOnly)
	}
	if got.Matched == nil || *got.Matched != primary {
		t.Fatalf("matched = %+v, want primary %+v", got.Matched, primary)
	}
	if got.MatchCount != 2 {
		t.Fatalf("match_count = %d, want 2", got.MatchCount)
	}
	wantMatches := []dedupMatchedCompany{primary, secondary}
	if !reflect.DeepEqual(got.Matches, wantMatches) {
		t.Fatalf("matches = %+v, want %+v", got.Matches, wantMatches)
	}
	if got.Name != "Acme, Inc." {
		t.Fatalf("name = %q, want trimmed input", got.Name)
	}
	if len(source.tokenCalls) != 0 {
		t.Fatalf("token lookup called without token: %+v", source.tokenCalls)
	}
	assertDedupNameCalls(t, source.nameCalls, []fakeDedupNameCall{{
		Names:       []string{"Acme, Inc."},
		RecencyDays: dedupDefaultRecencyDays,
	}})
}

func TestRunDedupCandidates_NoTokenOrNameMatchMapsToNew(t *testing.T) {
	source := &fakeDedupSource{}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
		Candidates: []dedupCandidateInput{{
			Name:       "Acme",
			ATS:        "greenhouse",
			BoardToken: "acme",
		}},
	}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if len(env.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(env.Results))
	}
	got := env.Results[0]
	if got.Verdict != dedupVerdictNew {
		t.Fatalf("verdict = %q, want %q", got.Verdict, dedupVerdictNew)
	}
	if got.MatchKind != dedupMatchKindNone {
		t.Fatalf("match_kind = %q, want %q", got.MatchKind, dedupMatchKindNone)
	}
	if got.Reason != dedupReasonNoMatch {
		t.Fatalf("reason = %q, want %q", got.Reason, dedupReasonNoMatch)
	}
	if got.Matched != nil {
		t.Fatalf("matched = %+v, want nil", got.Matched)
	}
	if got.MatchCount != 0 {
		t.Fatalf("match_count = %d, want 0", got.MatchCount)
	}
	if len(got.Matches) != 0 {
		t.Fatalf("matches = %+v, want empty", got.Matches)
	}
	assertDedupTokenCalls(t, source.tokenCalls, []fakeDedupTokenCall{{
		ATS:         "greenhouse",
		BoardToken:  "acme",
		RecencyDays: dedupDefaultRecencyDays,
	}})
	assertDedupNameCalls(t, source.nameCalls, []fakeDedupNameCall{{
		Names:       []string{"Acme"},
		RecencyDays: dedupDefaultRecencyDays,
	}})
}

func TestRunDedupCandidates_InvalidNameReturnsInvalidResultAndKeepsSiblings(t *testing.T) {
	match := dedupMatchedCompany{
		ID:                42,
		Name:              "Acme",
		ATS:               "greenhouse",
		BoardToken:        "acme",
		HasRecentSnapshot: true,
		MatchKind:         dedupMatchKindToken,
	}
	source := &fakeDedupSource{
		tokenMatches: map[string]dedupMatchedCompany{
			dedupTokenKey("greenhouse", "acme"): match,
		},
	}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
		Candidates: []dedupCandidateInput{
			{Name: "Acme", ATS: "greenhouse", BoardToken: "acme"},
			{Name: "   "},
			{Name: "NewCo"},
		},
	}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true for per-candidate validation; errors=%+v", env.Errors)
	}
	if len(env.Errors) != 0 {
		t.Fatalf("errors = %+v, want no top-level errors", env.Errors)
	}
	if len(env.Results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(env.Results))
	}
	if env.Results[0].Verdict != dedupVerdictDuplicate {
		t.Fatalf("results[0].verdict = %q, want %q", env.Results[0].Verdict, dedupVerdictDuplicate)
	}
	if env.Results[1].Verdict != dedupVerdictInvalid {
		t.Fatalf("results[1].verdict = %q, want %q", env.Results[1].Verdict, dedupVerdictInvalid)
	}
	if env.Results[1].MatchKind != dedupMatchKindNone {
		t.Fatalf("results[1].match_kind = %q, want %q", env.Results[1].MatchKind, dedupMatchKindNone)
	}
	if env.Results[1].Reason != dedupReasonMissingRequiredName {
		t.Fatalf("results[1].reason = %q, want %q", env.Results[1].Reason, dedupReasonMissingRequiredName)
	}
	if env.Results[1].Error == nil {
		t.Fatalf("results[1].error = nil, want per-candidate error")
	}
	if env.Results[1].MatchCount != 0 || len(env.Results[1].Matches) != 0 {
		t.Fatalf("results[1] matches = count %d rows %+v, want empty", env.Results[1].MatchCount, env.Results[1].Matches)
	}
	if got := *env.Results[1].Error; got.Path != "candidates[1].name" || got.Code != codeMissingRequired {
		t.Fatalf("results[1].error = %+v, want candidates[1].name/%s", got, codeMissingRequired)
	}
	if env.Results[2].Verdict != dedupVerdictNew {
		t.Fatalf("results[2].verdict = %q, want %q", env.Results[2].Verdict, dedupVerdictNew)
	}
	assertDedupNameCalls(t, source.nameCalls, []fakeDedupNameCall{{
		Names:       []string{"NewCo"},
		RecencyDays: dedupDefaultRecencyDays,
	}})
}

func TestRunDedupCandidates_RejectsOversizedBatchBeforeSource(t *testing.T) {
	source := &fakeDedupSource{}
	candidates := make([]dedupCandidateInput, dedupMaxCandidates+1)
	for i := range candidates {
		candidates[i].Name = fmt.Sprintf("Company %d", i)
	}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
		Candidates: candidates,
	}, source)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false")
	}
	if len(env.Results) != 0 {
		t.Fatalf("len(results) = %d, want 0 on call-level failure", len(env.Results))
	}
	if !hasError(env.Errors, "candidates", codeTooManyCandidates) {
		t.Fatalf("errors = %+v, want candidates/%s", env.Errors, codeTooManyCandidates)
	}
	if len(source.tokenCalls) != 0 {
		t.Fatalf("token lookup called for oversized batch: %+v", source.tokenCalls)
	}
	if len(source.nameCalls) != 0 {
		t.Fatalf("name lookup called for oversized batch: %+v", source.nameCalls)
	}
}

func TestRunDedupCandidates_DefaultRecencyDaysPassedToSource(t *testing.T) {
	source := &fakeDedupSource{}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
		Candidates: []dedupCandidateInput{{
			Name:       "Acme",
			ATS:        "greenhouse",
			BoardToken: "acme",
		}},
	}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	assertDedupTokenCalls(t, source.tokenCalls, []fakeDedupTokenCall{{
		ATS:         "greenhouse",
		BoardToken:  "acme",
		RecencyDays: dedupDefaultRecencyDays,
	}})
	assertDedupNameCalls(t, source.nameCalls, []fakeDedupNameCall{{
		Names:       []string{"Acme"},
		RecencyDays: dedupDefaultRecencyDays,
	}})
}

func TestRunDedupCandidates_RejectsNonPositiveRecencyDays(t *testing.T) {
	tests := []struct {
		name        string
		recencyDays int
	}{
		{"zero", 0},
		{"negative", -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &fakeDedupSource{}

			env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
				RecencyDays: &tc.recencyDays,
				Candidates: []dedupCandidateInput{{
					Name:       "Acme",
					ATS:        "greenhouse",
					BoardToken: "acme",
				}},
			}, source)

			if env.Ok {
				t.Fatalf("env.Ok = true, want false")
			}
			if len(env.Results) != 0 {
				t.Fatalf("len(results) = %d, want 0 on call-level failure", len(env.Results))
			}
			if !hasError(env.Errors, "recency_days", codeInvalidRecencyDays) {
				t.Fatalf("errors = %+v, want recency_days/%s", env.Errors, codeInvalidRecencyDays)
			}
			if len(source.tokenCalls) != 0 {
				t.Fatalf("token lookup called for invalid recency_days: %+v", source.tokenCalls)
			}
			if len(source.nameCalls) != 0 {
				t.Fatalf("name lookup called for invalid recency_days: %+v", source.nameCalls)
			}
		})
	}
}

func TestRunDedupCandidates_BatchesNameAndDomainIndependentlyThenSkipsFuzzy(t *testing.T) {
	source := &fakeDedupSource{
		nameMatches:   map[int][]dedupMatchedCompany{0: {{ID: 1, Name: "Same Name"}}},
		domainMatches: map[int][]dedupMatchedCompany{0: {{ID: 2, Name: "Domain Company"}}},
	}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{
		{Name: "Same Name", CareersURL: "https://www.example.com/jobs"},
		{Name: "No URL"},
	}}, source)

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	assertDedupNameCalls(t, source.nameCalls, []fakeDedupNameCall{{
		Names: []string{"Same Name", "No URL"}, RecencyDays: dedupDefaultRecencyDays,
	}})
	if !reflect.DeepEqual(source.domainCalls, []fakeDedupDomainCall{{
		CareersURLs: []string{"https://www.example.com/jobs"}, RecencyDays: dedupDefaultRecencyDays,
	}}) {
		t.Fatalf("domain calls = %+v", source.domainCalls)
	}
	if !reflect.DeepEqual(source.similarityCalls, []fakeDedupSimilarityCall{{
		Names: []string{"No URL"}, RecencyDays: dedupDefaultRecencyDays,
	}}) {
		t.Fatalf("similarity calls = %+v", source.similarityCalls)
	}
	got := env.Results[0]
	if got.MatchKind != dedupMatchKindDomain || got.Reason != dedupReasonMatchedByDomain || got.Verdict != dedupVerdictStale {
		t.Fatalf("result = %+v, want stale domain result", got)
	}
	if got.MatchCount != 2 || len(got.Matches) != 2 || got.Matched == nil || got.Matched.ID != 2 {
		t.Fatalf("result matches = %+v, want union with domain primary", got)
	}
	if got.Matches[0].MatchKind != dedupMatchKindNameOnly || got.Matches[1].MatchKind != dedupMatchKindDomain {
		t.Fatalf("match kinds = %+v, want stamped name/domain", got.Matches)
	}
}

func TestRunDedupCandidates_MergesSameCompanyAtStrongestPriority(t *testing.T) {
	source := &fakeDedupSource{
		nameMatches:   map[int][]dedupMatchedCompany{0: {{ID: 7, Name: "Acme by name"}}},
		domainMatches: map[int][]dedupMatchedCompany{0: {{ID: 7, Name: "Acme by domain"}}},
	}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{
		Name: "Acme", CareersURL: "https://acme.example/careers",
	}}}, source)

	got := env.Results[0]
	if got.MatchCount != 1 || len(got.Matches) != 1 {
		t.Fatalf("matches = count %d rows %+v, want one distinct company", got.MatchCount, got.Matches)
	}
	if got.Matches[0].MatchKind != dedupMatchKindDomain || got.Matches[0].Name != "Acme by domain" {
		t.Fatalf("match = %+v, want domain row retained", got.Matches[0])
	}
}

func TestRunDedupCandidates_InvalidCareersURLIgnoredWithoutBlockingOtherSignals(t *testing.T) {
	source := &fakeDedupSource{nameMatches: map[int][]dedupMatchedCompany{0: {{ID: 9, Name: "Acme"}}}}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{
		Name: "Acme", CareersURL: "://invalid",
	}}}, source)

	if !env.Ok || env.Results[0].MatchKind != dedupMatchKindNameOnly {
		t.Fatalf("env = %+v, want successful name match", env)
	}
	if len(source.domainCalls) != 0 {
		t.Fatalf("domain calls = %+v, want none for invalid URL", source.domainCalls)
	}
}

func TestRunDedupCandidates_FuzzyFallbackCapsReturnedRowsAfterTrueDistinctCount(t *testing.T) {
	scores := []float64{0.41, 0.92, 0.63, 0.81, 0.7}
	rows := make([]dedupMatchedCompany, len(scores))
	for i := range scores {
		rows[i] = dedupMatchedCompany{ID: int64(i + 1), Name: fmt.Sprintf("Acme %d", i), SimilarityScore: &scores[i]}
	}
	source := &fakeDedupSource{similarityMatches: map[int][]dedupMatchedCompany{0: rows}}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{Name: "Acme"}}}, source)

	got := env.Results[0]
	if got.Verdict != dedupVerdictStale || got.MatchKind != dedupMatchKindFuzzyName || got.Reason != dedupReasonMatchedByFuzzyName {
		t.Fatalf("result = %+v, want stale fuzzy result", got)
	}
	if got.MatchCount != 5 || len(got.Matches) != 3 {
		t.Fatalf("matches = count %d rows %d, want true count 5 and cap 3", got.MatchCount, len(got.Matches))
	}
	wantScores := []float64{0.92, 0.81, 0.7}
	for i, want := range wantScores {
		if got.Matches[i].MatchKind != dedupMatchKindFuzzyName || got.Matches[i].SimilarityScore == nil || *got.Matches[i].SimilarityScore != want {
			t.Fatalf("matches[%d] = %+v, want fuzzy score %v", i, got.Matches[i], want)
		}
	}
}

func TestRunDedupCandidates_TokenShortCircuitSkipsAllBatchSignals(t *testing.T) {
	source := &fakeDedupSource{tokenMatches: map[string]dedupMatchedCompany{
		dedupTokenKey("greenhouse", "acme"): {ID: 1, Name: "Acme", HasRecentSnapshot: true},
	}}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{
		Name: "Acme", ATS: "greenhouse", BoardToken: "acme", CareersURL: "https://acme.example/jobs",
	}}}, source)

	if env.Results[0].Verdict != dedupVerdictDuplicate || env.Results[0].Matches[0].MatchKind != dedupMatchKindToken {
		t.Fatalf("result = %+v, want token duplicate", env.Results[0])
	}
	if len(source.nameCalls)+len(source.domainCalls)+len(source.similarityCalls) != 0 {
		t.Fatalf("batch calls after token match: names=%+v domains=%+v fuzzy=%+v", source.nameCalls, source.domainCalls, source.similarityCalls)
	}
	if len(source.unsupportedNameCalls)+len(source.unsupportedDomainCalls) != 0 {
		t.Fatalf("unsupported lookups after token match: names=%+v domains=%+v", source.unsupportedNameCalls, source.unsupportedDomainCalls)
	}
}

func TestRunDedupCandidates_KnownUnsupportedIsIndependentSignal(t *testing.T) {
	lastCheckedAt := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	source := &fakeDedupSource{unsupportedNameMatches: map[int]dedupUnsupportedRecord{
		0: {
			Name:             "Unsupported Acme",
			Reason:           unsupportedCompanyReasonUnsupportedATS,
			FirstSeenAt:      "2026-01-01T00:00:00Z",
			LastCheckedAt:    lastCheckedAt,
			DetectedPlatform: dedupTestStringPtr("rippling"),
		},
	}}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{Name: "Unsupported Acme"}}}, source)
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	got := env.Results[0]
	if got.Verdict != dedupVerdictNew || got.MatchKind != dedupMatchKindNone || got.Reason != dedupReasonNoMatch || got.MatchCount != 0 || got.Matched != nil || len(got.Matches) != 0 {
		t.Fatalf("dedup result = %+v, want unchanged new/no-match result", got)
	}
	if got.KnownUnsupported == nil {
		t.Fatal("known_unsupported = nil, want populated independent signal")
	}
	if got.KnownUnsupported.Name != "Unsupported Acme" || got.KnownUnsupported.DetectedPlatform == nil || *got.KnownUnsupported.DetectedPlatform != "rippling" || got.KnownUnsupported.Stale {
		t.Fatalf("known_unsupported = %+v, want current registry record", got.KnownUnsupported)
	}
}

func TestRunDedupCandidates_KnownUnsupportedHostWinsOverName(t *testing.T) {
	source := &fakeDedupSource{
		unsupportedNameMatches: map[int]dedupUnsupportedRecord{0: {
			Name:          "Name Match",
			Reason:        unsupportedCompanyReasonNoCareers,
			FirstSeenAt:   "2026-01-01T00:00:00Z",
			LastCheckedAt: time.Now().UTC().Format(time.RFC3339),
		}},
		unsupportedDomainMatches: map[int]dedupUnsupportedRecord{0: {
			Name:          "Host Match",
			Reason:        unsupportedCompanyReasonUnsupportedATS,
			FirstSeenAt:   "2026-01-02T00:00:00Z",
			LastCheckedAt: time.Now().UTC().Format(time.RFC3339),
		}},
	}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{
		Name: "Name Match", CareersURL: "https://unsupported.example/careers",
	}}}, source)
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if got := env.Results[0].KnownUnsupported; got == nil || got.Name != "Host Match" || got.Reason != unsupportedCompanyReasonUnsupportedATS {
		t.Fatalf("known_unsupported = %+v, want host match", got)
	}
}

func TestRunDedupCandidates_KnownUnsupportedDoesNotChangeExistingMatch(t *testing.T) {
	companyMatch := dedupMatchedCompany{ID: 17, Name: "Tracked Acme"}
	source := &fakeDedupSource{
		nameMatches: map[int][]dedupMatchedCompany{0: {companyMatch}},
		unsupportedNameMatches: map[int]dedupUnsupportedRecord{0: {
			Name: "Unsupported Acme", Reason: unsupportedCompanyReasonNoCareers,
			FirstSeenAt: "2026-01-01T00:00:00Z", LastCheckedAt: time.Now().UTC().Format(time.RFC3339),
		}},
	}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{Name: "Acme"}}}, source)
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	got := env.Results[0]
	if got.Verdict != dedupVerdictStale || got.MatchKind != dedupMatchKindNameOnly || got.Reason != dedupReasonMatchedByNameOnly || got.MatchCount != 1 || got.Matched == nil || got.Matched.ID != companyMatch.ID || got.Matched.MatchKind != dedupMatchKindNameOnly {
		t.Fatalf("dedup result = %+v, want unchanged name-match result", got)
	}
	if got.KnownUnsupported == nil || got.KnownUnsupported.Name != "Unsupported Acme" {
		t.Fatalf("known_unsupported = %+v, want independent registry signal", got.KnownUnsupported)
	}
}

func TestRunDedupCandidates_KnownUnsupportedStaleAfterRevisitThreshold(t *testing.T) {
	now := time.Now().UTC()
	source := &fakeDedupSource{unsupportedNameMatches: map[int]dedupUnsupportedRecord{
		0: {
			Name: "Current", Reason: unsupportedCompanyReasonNoCareers,
			FirstSeenAt: "2026-01-01T00:00:00Z", LastCheckedAt: now.Add(-dedupUnsupportedRevisitDays*24*time.Hour + time.Hour).Format(time.RFC3339),
		},
		1: {
			Name: "Stale", Reason: unsupportedCompanyReasonNoCareers,
			FirstSeenAt: "2026-01-01T00:00:00Z", LastCheckedAt: now.Add(-dedupUnsupportedRevisitDays*24*time.Hour - time.Hour).Format(time.RFC3339),
		},
	}}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{{Name: "Current"}, {Name: "Stale"}}}, source)
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.Results[0].KnownUnsupported == nil || env.Results[0].KnownUnsupported.Stale {
		t.Fatalf("current record = %+v, want stale=false", env.Results[0].KnownUnsupported)
	}
	if env.Results[1].KnownUnsupported == nil || !env.Results[1].KnownUnsupported.Stale {
		t.Fatalf("stale record = %+v, want stale=true", env.Results[1].KnownUnsupported)
	}
}

func TestRunDedupCandidates_EachBatchLookupErrorFailsTheCall(t *testing.T) {
	tests := []struct {
		name   string
		source *fakeDedupSource
		input  dedupCandidateInput
	}{
		{"name", &fakeDedupSource{nameErr: errors.New("name failed")}, dedupCandidateInput{Name: "Acme"}},
		{"domain", &fakeDedupSource{domainErr: errors.New("domain failed")}, dedupCandidateInput{Name: "Acme", CareersURL: "https://acme.example/jobs"}},
		{"unsupported name", &fakeDedupSource{unsupportedNameErr: errors.New("unsupported name failed")}, dedupCandidateInput{Name: "Acme"}},
		{"unsupported domain", &fakeDedupSource{unsupportedDomainErr: errors.New("unsupported domain failed")}, dedupCandidateInput{Name: "Acme", CareersURL: "https://acme.example/jobs"}},
		{"fuzzy", &fakeDedupSource{similarityErr: errors.New("fuzzy failed")}, dedupCandidateInput{Name: "Acme"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := runDedupCandidates(t.Context(), dedupCandidatesRequest{Candidates: []dedupCandidateInput{tc.input}}, tc.source)
			if env.Ok || len(env.Results) != 0 || !hasError(env.Errors, "db", codeDBError) {
				t.Fatalf("env = %+v, want call-level DB failure", env)
			}
		})
	}
}

func TestDedupCandidatesHandlerWithDeps_ResponseMapping(t *testing.T) {
	recencyDays := 12
	match := dedupMatchedCompany{
		ID:                101,
		Name:              "Example Co",
		ATS:               "workday",
		BoardToken:        "example.wd5.myworkdayjobs.com/Careers",
		HasRecentSnapshot: false,
		MatchKind:         dedupMatchKindToken,
	}
	source := &fakeDedupSource{
		tokenMatches: map[string]dedupMatchedCompany{
			dedupTokenKey("workday", "example.wd5.myworkdayjobs.com/Careers"): match,
		},
	}

	env := callDedupCandidates(t, dedupCandidatesHandlerWithDeps(source), newDedupCallRequest(map[string]any{
		"recency_days": recencyDays,
		"candidates": []map[string]any{{
			"name":        " Example Co ",
			"ats":         "workday",
			"board_token": "example.wd5.myworkdayjobs.com/Careers",
		}},
	}))

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if len(env.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(env.Results))
	}
	got := env.Results[0]
	if got.Name != "Example Co" || got.ATS != "workday" || got.BoardToken != "example.wd5.myworkdayjobs.com/Careers" {
		t.Fatalf("result echo = %+v, want trimmed candidate fields", got)
	}
	if got.Verdict != dedupVerdictStale {
		t.Fatalf("verdict = %q, want %q", got.Verdict, dedupVerdictStale)
	}
	if got.MatchKind != dedupMatchKindToken {
		t.Fatalf("match_kind = %q, want %q", got.MatchKind, dedupMatchKindToken)
	}
	if got.Reason != dedupReasonMatchedByTokenStale {
		t.Fatalf("reason = %q, want %q", got.Reason, dedupReasonMatchedByTokenStale)
	}
	if got.Matched == nil || *got.Matched != match {
		t.Fatalf("matched = %+v, want %+v", got.Matched, match)
	}
	if got.MatchCount != 1 {
		t.Fatalf("match_count = %d, want 1", got.MatchCount)
	}
	if !reflect.DeepEqual(got.Matches, []dedupMatchedCompany{match}) {
		t.Fatalf("matches = %+v, want primary token match", got.Matches)
	}
	assertDedupTokenCalls(t, source.tokenCalls, []fakeDedupTokenCall{{
		ATS:         "workday",
		BoardToken:  "example.wd5.myworkdayjobs.com/Careers",
		RecencyDays: int32(recencyDays),
	}})
}

func TestRunDedupCandidates_DBErrorReturnsCallLevelEnvelope(t *testing.T) {
	source := &fakeDedupSource{tokenErr: errors.New("connection refused")}

	env := runDedupCandidates(t.Context(), dedupCandidatesRequest{
		Candidates: []dedupCandidateInput{{Name: "Acme", ATS: "greenhouse", BoardToken: "acme"}},
	}, source)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false on source error")
	}
	if len(env.Results) != 0 {
		t.Fatalf("len(results) = %d, want 0 on call-level failure", len(env.Results))
	}
	if !hasError(env.Errors, "db", codeDBError) {
		t.Fatalf("errors = %+v, want db/%s", env.Errors, codeDBError)
	}
}

func newDedupCallRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func callDedupCandidates(t *testing.T, handler server.ToolHandlerFunc, req mcp.CallToolRequest) dedupCandidatesEnvelope {
	t.Helper()
	res, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if res == nil || len(res.Content) != 1 {
		t.Fatalf("handler result = %+v, want exactly one content block", res)
	}
	text, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("handler content = %T, want TextContent", res.Content[0])
	}
	var env dedupCandidatesEnvelope
	if err := json.Unmarshal([]byte(text.Text), &env); err != nil {
		t.Fatalf("decoding dedup_candidates envelope %q: %v", text.Text, err)
	}
	return env
}

func assertDedupTokenCalls(t *testing.T, got, want []fakeDedupTokenCall) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("token calls = %+v, want %+v", got, want)
	}
}

func assertDedupNameCalls(t *testing.T, got, want []fakeDedupNameCall) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("name calls = %s, want %s", formatDedupNameCalls(got), formatDedupNameCalls(want))
	}
}

func formatDedupNameCalls(calls []fakeDedupNameCall) string {
	payload, err := json.Marshal(calls)
	if err != nil {
		return fmt.Sprintf("%+v", calls)
	}
	return string(payload)
}

func dedupTestStringPtr(value string) *string {
	return &value
}
