package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

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

type fakeDedupSource struct {
	tokenMatches      map[string]dedupMatchedCompany
	nameMatches       map[int][]dedupMatchedCompany
	similarityMatches map[int][]dedupMatchedCompany
	domainMatches     map[int][]dedupMatchedCompany
	tokenErr          error
	nameErr           error
	similarityErr     error
	domainErr         error
	tokenCalls        []fakeDedupTokenCall
	nameCalls         []fakeDedupNameCall
	similarityCalls   []fakeDedupSimilarityCall
	domainCalls       []fakeDedupDomainCall
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
	}
	secondary := dedupMatchedCompany{
		ID:                93,
		Name:              "A.C.M.E. Inc",
		ATS:               "greenhouse",
		BoardToken:        "acme",
		HasRecentSnapshot: false,
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

func TestDedupCandidatesHandlerWithDeps_ResponseMapping(t *testing.T) {
	recencyDays := 12
	match := dedupMatchedCompany{
		ID:                101,
		Name:              "Example Co",
		ATS:               "workday",
		BoardToken:        "example.wd5.myworkdayjobs.com/Careers",
		HasRecentSnapshot: false,
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
