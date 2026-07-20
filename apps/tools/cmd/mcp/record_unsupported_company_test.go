package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRecordUnsupportedCompanyExecutor struct {
	row    recordUnsupportedCompanyRow
	err    error
	called bool
	params recordUnsupportedCompanyParams
}

func (f *fakeRecordUnsupportedCompanyExecutor) recordUnsupportedCompany(_ context.Context, params recordUnsupportedCompanyParams) (recordUnsupportedCompanyRow, error) {
	f.called = true
	f.params = params
	if f.err != nil {
		return recordUnsupportedCompanyRow{}, f.err
	}
	return f.row, nil
}

func TestRunRecordUnsupportedCompany_Validation(t *testing.T) {
	tests := []struct {
		name     string
		req      recordUnsupportedCompanyRequest
		wantPath string
		wantCode string
	}{
		{"missing name", recordUnsupportedCompanyRequest{Reason: unsupportedCompanyReasonNoCareers}, "name", codeMissingRequired},
		{"missing reason", recordUnsupportedCompanyRequest{Name: "Acme"}, "reason", codeMissingRequired},
		{"invalid reason", recordUnsupportedCompanyRequest{Name: "Acme", Reason: "invalid"}, "reason", codeInvalidUnsupportedCompanyReason},
		{"unsupported ats needs url", recordUnsupportedCompanyRequest{Name: "Acme", Reason: unsupportedCompanyReasonUnsupportedATS}, "url", codeMissingRequired},
		{"invalid supplied url", recordUnsupportedCompanyRequest{Name: "Acme", Reason: unsupportedCompanyReasonNoCareers, URL: "not a url"}, "url", codeInvalidURL},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec := &fakeRecordUnsupportedCompanyExecutor{}
			env := runRecordUnsupportedCompany(t.Context(), tc.req, exec)

			if env.Ok {
				t.Fatalf("env.Ok = true, want false")
			}
			if exec.called {
				t.Fatalf("executor called on validation failure")
			}
			if !hasError(env.Errors, tc.wantPath, tc.wantCode) {
				t.Fatalf("errors = %+v, want path=%q code=%q", env.Errors, tc.wantPath, tc.wantCode)
			}
		})
	}
}

func TestRunRecordUnsupportedCompany_MapsRowAndBindsNullOptionals(t *testing.T) {
	firstSeen := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	lastChecked := firstSeen.Add(time.Hour)
	exec := &fakeRecordUnsupportedCompanyExecutor{row: recordUnsupportedCompanyRow{
		ID:            17,
		Name:          "Acme",
		Reason:        unsupportedCompanyReasonNoCareers,
		FirstSeenAt:   firstSeen,
		LastCheckedAt: lastChecked,
	}}

	env := runRecordUnsupportedCompany(t.Context(), recordUnsupportedCompanyRequest{
		Name:   " Acme ",
		Reason: unsupportedCompanyReasonNoCareers,
	}, exec)

	if !env.Ok || env.UnsupportedCompany == nil {
		t.Fatalf("env = %+v, want successful company", env)
	}
	if env.UnsupportedCompany.FirstSeenAt != "2026-07-20T12:00:00Z" || env.UnsupportedCompany.LastCheckedAt != "2026-07-20T13:00:00Z" {
		t.Fatalf("timestamps = %q / %q, want RFC3339 values", env.UnsupportedCompany.FirstSeenAt, env.UnsupportedCompany.LastCheckedAt)
	}
	if exec.params.Name != "Acme" || exec.params.Reason != unsupportedCompanyReasonNoCareers {
		t.Fatalf("params = %+v, want trimmed name and reason", exec.params)
	}
	if exec.params.URL.Valid || exec.params.DetectedPlatform.Valid {
		t.Fatalf("optional params = %+v, want SQL NULL values", exec.params)
	}
}

func TestRunRecordUnsupportedCompany_DBErrorReturnsEnvelope(t *testing.T) {
	exec := &fakeRecordUnsupportedCompanyExecutor{err: errors.New("connection reset")}
	env := runRecordUnsupportedCompany(t.Context(), recordUnsupportedCompanyRequest{
		Name:   "Acme",
		Reason: unsupportedCompanyReasonNoCareers,
	}, exec)

	if env.Ok {
		t.Fatalf("env.Ok = true, want false")
	}
	if !hasError(env.Errors, "db", codeDBError) {
		t.Fatalf("errors = %+v, want db_error", env.Errors)
	}
}
