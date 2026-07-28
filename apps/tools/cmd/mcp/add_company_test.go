package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/domain"
)

// fakeExecutor returns canned rows (or an error) instead of touching a database.
type fakeExecutor struct {
	row    addCompanyRow
	err    error
	called bool
}

func (f *fakeExecutor) addCompany(ctx context.Context, params addCompanyParams) (addCompanyRow, error) {
	f.called = true
	if f.err != nil {
		return addCompanyRow{}, f.err
	}
	return f.row, nil
}

// fakeProbe records whether FetchPostings ran and returns canned postings/error.
type fakeProbe struct {
	postings []domain.Posting
	err      error
	called   bool
}

func (f *fakeProbe) FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error) {
	f.called = true
	return f.postings, f.err
}

// boolPtr is a local helper for the *bool probe field.
func boolPtr(b bool) *bool { return &b }

// neverProbe fails the test if the probe factory is invoked. Used to assert a
// validation failure aborts before any probe.
func neverProbe(t *testing.T) probeFactory {
	return func(string) (atsProbe, error) {
		t.Helper()
		t.Fatalf("probe factory called, want no probe")
		return nil, nil
	}
}

func TestRunAddCompany_ValidationErrorCodes(t *testing.T) {
	tests := []struct {
		name     string
		req      addCompanyRequest
		wantPath string
		wantCode string
	}{
		{
			name:     "unsupported ats",
			req:      addCompanyRequest{Name: "Acme", ATS: "rippling", BoardToken: "acme", Probe: boolPtr(false)},
			wantPath: "ats",
			wantCode: codeUnsupportedATS,
		},
		{
			name:     "missing name after trim",
			req:      addCompanyRequest{Name: "   ", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)},
			wantPath: "name",
			wantCode: codeMissingRequired,
		},
		{
			name:     "missing board_token after trim",
			req:      addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "   ", Probe: boolPtr(false)},
			wantPath: "board_token",
			wantCode: codeMissingRequired,
		},
		{
			name:     "malformed workday token",
			req:      addCompanyRequest{Name: "Acme", ATS: "workday", BoardToken: "acme.example.com/Careers", Probe: boolPtr(false)},
			wantPath: "board_token",
			wantCode: codeInvalidBoardToken,
		},
		{
			name:     "workday token missing site",
			req:      addCompanyRequest{Name: "Acme", ATS: "workday", BoardToken: "acme.wd5.myworkdayjobs.com", Probe: boolPtr(false)},
			wantPath: "board_token",
			wantCode: codeInvalidBoardToken,
		},
		{
			name:     "workday host with leading dot rejected",
			req:      addCompanyRequest{Name: "Acme", ATS: "workday", BoardToken: ".myworkdayjobs.com/Careers", Probe: boolPtr(false)},
			wantPath: "board_token",
			wantCode: codeInvalidBoardToken,
		},
		{
			name:     "workday site with query separator rejected",
			req:      addCompanyRequest{Name: "Acme", ATS: "workday", BoardToken: "acme.wd5.myworkdayjobs.com/Careers?foo", Probe: boolPtr(false)},
			wantPath: "board_token",
			wantCode: codeInvalidBoardToken,
		},
		{
			name:     "malformed workable token",
			req:      addCompanyRequest{Name: "Acme", ATS: "workable", BoardToken: "Acme_Co", Probe: boolPtr(false)},
			wantPath: "board_token",
			wantCode: codeInvalidBoardToken,
		},
		{
			name:     "invalid careers url",
			req:      addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", CareersPageURL: "not a url", Probe: boolPtr(false)},
			wantPath: "careers_page_url",
			wantCode: codeInvalidURL,
		},
		{
			name:     "bare careers url rejected",
			req:      addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", CareersPageURL: "example.com/careers", Probe: boolPtr(false)},
			wantPath: "careers_page_url",
			wantCode: codeInvalidURL,
		},
		{
			name:     "non-http careers url rejected",
			req:      addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", CareersPageURL: "ftp://example.com/careers", Probe: boolPtr(false)},
			wantPath: "careers_page_url",
			wantCode: codeInvalidURL,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec := &fakeExecutor{}
			env := runAddCompany(t.Context(), tc.req, exec, neverProbe(t))

			if env.Ok {
				t.Fatalf("env.Ok = true, want false for validation failure")
			}
			if exec.called {
				t.Fatalf("executor called on validation failure, want no insert")
			}
			if !hasError(env.Errors, tc.wantPath, tc.wantCode) {
				t.Fatalf("errors = %+v, want path=%q code=%q", env.Errors, tc.wantPath, tc.wantCode)
			}
		})
	}
}

func TestRunAddCompany_CareersPageURLValidationKeepsFieldMessage(t *testing.T) {
	exec := &fakeExecutor{}
	req := addCompanyRequest{
		Name:           "Acme",
		ATS:            "greenhouse",
		BoardToken:     "acme",
		CareersPageURL: "not a url",
		Probe:          boolPtr(false),
	}

	env := runAddCompany(t.Context(), req, exec, neverProbe(t))

	if env.Ok {
		t.Fatalf("env.Ok = true, want false for invalid careers_page_url")
	}
	if len(env.Errors) != 1 {
		t.Fatalf("errors = %+v, want exactly one careers_page_url error", env.Errors)
	}
	got := env.Errors[0]
	if got.Path != "careers_page_url" || got.Code != codeInvalidURL {
		t.Fatalf("error = %+v, want careers_page_url/%s", got, codeInvalidURL)
	}
	if !strings.HasPrefix(got.Message, "careers_page_url ") {
		t.Fatalf("message = %q, want field-specific careers_page_url wording", got.Message)
	}
}

func TestRunAddCompany_ValidWorkdayAndWorkableTokensPass(t *testing.T) {
	tests := []struct {
		name  string
		atsID string
		token string
	}{
		{"workday", "workday", "acme.wd5.myworkdayjobs.com/Careers"},
		{"workable", "workable", "acme-co"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec := &fakeExecutor{row: sampleRow(true)}
			req := addCompanyRequest{Name: "Acme", ATS: tc.atsID, BoardToken: tc.token, Probe: boolPtr(false)}
			env := runAddCompany(t.Context(), req, exec, neverProbe(t))
			if !env.Ok {
				t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
			}
			if !exec.called {
				t.Fatalf("executor not called, want insert")
			}
		})
	}
}

func TestRunAddCompany_ProbeDefaultsToTrueWhenOmitted(t *testing.T) {
	probe := &fakeProbe{}
	exec := &fakeExecutor{row: sampleRow(true)}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme"} // Probe nil

	env := runAddCompany(t.Context(), req, exec, func(string) (atsProbe, error) { return probe, nil })

	if !probe.called {
		t.Fatalf("probe not called when probe omitted, want probe attempted by default")
	}
	if env.ProbeResult == nil || !env.ProbeResult.Attempted {
		t.Fatalf("probe_result.attempted = false, want true")
	}
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
}

func TestRunAddCompany_ProbeFalseSkipsProbe(t *testing.T) {
	exec := &fakeExecutor{row: sampleRow(true)}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)}

	env := runAddCompany(t.Context(), req, exec, neverProbe(t))

	if env.ProbeResult == nil || env.ProbeResult.Attempted {
		t.Fatalf("probe_result.attempted = true, want false when probe=false")
	}
	if !exec.called {
		t.Fatalf("executor not called, want insert when probe=false")
	}
}

func TestRunAddCompany_ProbeFailureAbortsInsert(t *testing.T) {
	probe := &fakeProbe{err: errors.New("board not found")}
	exec := &fakeExecutor{row: sampleRow(true)}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme"}

	env := runAddCompany(t.Context(), req, exec, func(string) (atsProbe, error) { return probe, nil })

	if env.Ok {
		t.Fatalf("env.Ok = true, want false on probe failure")
	}
	if exec.called {
		t.Fatalf("executor called on probe failure, want no insert")
	}
	if !hasError(env.Errors, "probe", codeProbeFailed) {
		t.Fatalf("errors = %+v, want probe_failed", env.Errors)
	}
	if env.ProbeResult == nil || env.ProbeResult.Valid {
		t.Fatalf("probe_result valid = true, want failed probe surfaced")
	}
}

func TestRunAddCompany_EmptyBoardIsValid(t *testing.T) {
	probe := &fakeProbe{postings: nil} // empty board
	exec := &fakeExecutor{row: sampleRow(true)}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme"}

	env := runAddCompany(t.Context(), req, exec, func(string) (atsProbe, error) { return probe, nil })

	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; empty board is valid")
	}
	if env.ProbeResult.PostingsCount != 0 {
		t.Fatalf("postings_count = %d, want 0", env.ProbeResult.PostingsCount)
	}
}

func TestRunAddCompany_DBErrorReturnsEnvelope(t *testing.T) {
	exec := &fakeExecutor{err: errors.New("conn reset")}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)}

	env := runAddCompany(t.Context(), req, exec, neverProbe(t))

	if env.Ok {
		t.Fatalf("env.Ok = true, want false on db error")
	}
	if !hasError(env.Errors, "db", codeDBError) {
		t.Fatalf("errors = %+v, want db_error", env.Errors)
	}
}

// TestRunAddCompany_MapsInsertedRow checks the inserted=true mapping including
// the eight columns and non-null optional fields.
func TestRunAddCompany_MapsInsertedRow(t *testing.T) {
	created := time.Date(2026, 6, 22, 17, 8, 0, 0, time.UTC)
	exec := &fakeExecutor{row: addCompanyRow{
		ID:             7,
		Name:           "Acme",
		ATS:            "greenhouse",
		BoardToken:     "acme",
		CreatedAt:      sql.NullTime{Time: created, Valid: true},
		Industry:       sql.NullString{String: "fintech", Valid: true},
		CareersPageURL: sql.NullString{String: "https://acme.example/careers", Valid: true},
		Inserted:       true,
	}}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Industry: "fintech", CareersPageURL: "https://acme.example/careers", Probe: boolPtr(false)}

	env := runAddCompany(t.Context(), req, exec, neverProbe(t))

	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"ok":true,"inserted":true,"company":{"id":7,"name":"Acme","ats":"greenhouse","board_token":"acme","created_at":"2026-06-22T17:08:00Z","industry":"fintech","careers_page_url":"https://acme.example/careers"},"seed_file_updated":false,"follow_up":"Company row written to the database but not yet reflected in the canonical seed file apps/tools/internal/db/seeds/companies.sql. A human-reviewed step is needed to keep the seed authoritative; a future stage_company_seed_patch action will draft that seed SQL for review.","probe_result":{"ats":"greenhouse","board_token":"acme","attempted":false,"valid":false,"postings_count":0},"errors":[]}`
	if string(payload) != want {
		t.Fatalf("payload:\n got: %s\nwant: %s", payload, want)
	}
}

// TestRunAddCompany_MapsConflictRowWithNullOptionals checks inserted=false plus
// null industry/careers_page_url mapping (the on-conflict no-op path).
func TestRunAddCompany_MapsConflictRowWithNullOptionals(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	exec := &fakeExecutor{row: addCompanyRow{
		ID:             7,
		Name:           "Acme",
		ATS:            "greenhouse",
		BoardToken:     "acme",
		CreatedAt:      sql.NullTime{Time: created, Valid: true},
		Industry:       sql.NullString{},
		CareersPageURL: sql.NullString{},
		Inserted:       false,
	}}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)}

	env := runAddCompany(t.Context(), req, exec, neverProbe(t))

	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"ok":true,"inserted":false,"company":{"id":7,"name":"Acme","ats":"greenhouse","board_token":"acme","created_at":"2026-01-02T03:04:05Z","industry":null,"careers_page_url":null},"seed_file_updated":false,"follow_up":"Company row written to the database but not yet reflected in the canonical seed file apps/tools/internal/db/seeds/companies.sql. A human-reviewed step is needed to keep the seed authoritative; a future stage_company_seed_patch action will draft that seed SQL for review.","probe_result":{"ats":"greenhouse","board_token":"acme","attempted":false,"valid":false,"postings_count":0},"errors":[]}`
	if string(payload) != want {
		t.Fatalf("payload:\n got: %s\nwant: %s", payload, want)
	}
}

// TestRunAddCompany_SeedDriftFollowUp asserts that every successful outcome
// (insert and on-conflict no-op) surfaces the seed-drift follow-up naming the
// canonical seed file, while seed_file_updated stays false. Failure paths must
// leave follow_up null. See agent-context/lib/watchlist.md (seed is canonical).
func TestRunAddCompany_SeedDriftFollowUp(t *testing.T) {
	t.Run("successful insert names seed file", func(t *testing.T) {
		exec := &fakeExecutor{row: sampleRow(true)}
		req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)}

		env := runAddCompany(t.Context(), req, exec, neverProbe(t))

		assertSeedDriftFollowUp(t, env)
	})

	t.Run("successful no-op names seed file", func(t *testing.T) {
		exec := &fakeExecutor{row: sampleRow(false)}
		req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)}

		env := runAddCompany(t.Context(), req, exec, neverProbe(t))

		if env.Inserted {
			t.Fatalf("inserted = true, want false for no-op path")
		}
		assertSeedDriftFollowUp(t, env)
	})

	t.Run("validation failure leaves follow_up null", func(t *testing.T) {
		exec := &fakeExecutor{}
		req := addCompanyRequest{Name: "Acme", ATS: "rippling", BoardToken: "acme", Probe: boolPtr(false)}

		env := runAddCompany(t.Context(), req, exec, neverProbe(t))

		if env.Ok {
			t.Fatalf("env.Ok = true, want false for validation failure")
		}
		if env.FollowUp != nil {
			t.Fatalf("follow_up = %q, want null on failure", *env.FollowUp)
		}
		if env.SeedFileUpdated {
			t.Fatalf("seed_file_updated = true, want false")
		}
	})

	t.Run("db failure leaves follow_up null", func(t *testing.T) {
		exec := &fakeExecutor{err: errors.New("conn reset")}
		req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)}

		env := runAddCompany(t.Context(), req, exec, neverProbe(t))

		if env.Ok {
			t.Fatalf("env.Ok = true, want false on db error")
		}
		if env.FollowUp != nil {
			t.Fatalf("follow_up = %q, want null on failure", *env.FollowUp)
		}
		if env.SeedFileUpdated {
			t.Fatalf("seed_file_updated = true, want false")
		}
	})
}

// assertSeedDriftFollowUp checks the shared success contract: ok, a non-null
// follow_up naming the seed file, and seed_file_updated false.
func assertSeedDriftFollowUp(t *testing.T, env addCompanyEnvelope) {
	t.Helper()
	if !env.Ok {
		t.Fatalf("env.Ok = false, want true; errors=%+v", env.Errors)
	}
	if env.SeedFileUpdated {
		t.Fatalf("seed_file_updated = true, want false")
	}
	if env.FollowUp == nil {
		t.Fatalf("follow_up = null, want a seed-drift message")
	}
	if !strings.Contains(*env.FollowUp, seedFilePath) {
		t.Fatalf("follow_up = %q, want it to name %q", *env.FollowUp, seedFilePath)
	}
}

// TestRunAddCompany_OmittedOptionalsPassSQLNull verifies that omitted industry
// and careers_page_url are bound as SQL NULL, not empty strings.
func TestRunAddCompany_OmittedOptionalsPassSQLNull(t *testing.T) {
	capture := &paramCapture{row: sampleRow(true)}
	req := addCompanyRequest{Name: "Acme", ATS: "greenhouse", BoardToken: "acme", Probe: boolPtr(false)}

	runAddCompany(t.Context(), req, capture, neverProbe(t))

	if capture.params.Industry.Valid {
		t.Fatalf("industry param Valid = true, want NULL for omitted value")
	}
	if capture.params.CareersPageURL.Valid {
		t.Fatalf("careers_page_url param Valid = true, want NULL for omitted value")
	}
}

// TestRunAddCompany_NormalizesBoardTokenBeforeInsert verifies the write
// boundary applies atsdetect.NormalizeBoardToken to a caller-supplied
// board_token before it reaches the probe and the insert, since add_company
// accepts ats/board_token directly and callers can bypass detect_ats (which
// already normalizes) entirely.
func TestRunAddCompany_NormalizesBoardTokenBeforeInsert(t *testing.T) {
	tests := []struct {
		name       string
		ats        string
		boardToken string
		want       string
	}{
		{name: "greenhouse lowercases", ats: "greenhouse", boardToken: "Stripe", want: "stripe"},
		{name: "ashby lowercases", ats: "ashby", boardToken: "QAWolf", want: "qawolf"},
		{
			name:       "lever preserves case (case-sensitive API)",
			ats:        "lever",
			boardToken: "MastReforestation",
			want:       "MastReforestation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := &paramCapture{row: sampleRow(true)}
			req := addCompanyRequest{Name: "Acme", ATS: tc.ats, BoardToken: tc.boardToken, Probe: boolPtr(false)}

			runAddCompany(t.Context(), req, capture, neverProbe(t))

			if capture.params.BoardToken != tc.want {
				t.Fatalf("board_token param = %q, want %q", capture.params.BoardToken, tc.want)
			}
		})
	}
}

// paramCapture records the params passed to addCompany so tests can assert the
// NULL binding contract.
type paramCapture struct {
	params addCompanyParams
	row    addCompanyRow
}

func (p *paramCapture) addCompany(ctx context.Context, params addCompanyParams) (addCompanyRow, error) {
	p.params = params
	return p.row, nil
}

func sampleRow(inserted bool) addCompanyRow {
	return addCompanyRow{
		ID:         1,
		Name:       "Acme",
		ATS:        "greenhouse",
		BoardToken: "acme",
		CreatedAt:  sql.NullTime{Time: time.Unix(0, 0).UTC(), Valid: true},
		Inserted:   inserted,
	}
}

func hasError(errs []actionError, path, code string) bool {
	for _, e := range errs {
		if e.Path == path && e.Code == code {
			return true
		}
	}
	return false
}
