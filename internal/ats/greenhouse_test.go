package ats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestGreenhouseAdapter_SuccessfulParse(t *testing.T) {
	fixture := loadFixture(t, "jobs_full.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/exampleco/jobs"; got != want {
			t.Errorf("path: got %q, want %q", got, want)
		}
		if got := r.URL.Query().Get("content"); got != "true" {
			t.Errorf("content query: got %q, want %q", got, "true")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	adapter := newWithBaseURL(srv.Client(), srv.URL)
	postings, err := adapter.FetchPostings(t.Context(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 2 {
		t.Fatalf("got %d postings, want 2", len(postings))
	}
	p := postings[0]

	if p.SourceID != "5822886004" {
		t.Errorf("SourceID: got %q, want %q", p.SourceID, "5822886004")
	}
	wantSourceURL := srv.URL + "/exampleco/jobs/5822886004"
	if p.SourceURL != wantSourceURL {
		t.Errorf("SourceURL: got %q, want %q", p.SourceURL, wantSourceURL)
	}
	wantTitle := "Accenture Account Executive, Strategic (London, United Kingdom)  "
	if p.Title == nil || *p.Title != wantTitle {
		t.Errorf("Title: got %v, want pointer to %q", p.Title, wantTitle)
	}
	if p.LocationText == nil || *p.LocationText != "London, England" {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, "London, England")
	}
	if p.Department == nil || *p.Department != "Sales" {
		t.Errorf("Department: got %v, want pointer to %q", p.Department, "Sales")
	}
	if p.Team != nil {
		t.Errorf("Team: got %v, want nil", *p.Team)
	}
	if p.EmploymentType != nil {
		t.Errorf("EmploymentType: got %v, want nil", *p.EmploymentType)
	}
	if p.WorkplaceType != nil {
		t.Errorf("WorkplaceType: got %v, want nil", *p.WorkplaceType)
	}
	if p.PostedAt != nil {
		t.Errorf("PostedAt: got %v, want nil (Greenhouse adapter never sets PostedAt)", *p.PostedAt)
	}
	wantJobURL := "https://boards.greenhouse.io/figma/jobs/5822886004?gh_jid=5822886004"
	if p.JobURL == nil || *p.JobURL != wantJobURL {
		t.Errorf("JobURL: got %v, want pointer to %q", p.JobURL, wantJobURL)
	}

	var rawCheck map[string]json.RawMessage
	if err := json.Unmarshal(p.RawData, &rawCheck); err != nil {
		t.Fatalf("RawData is not valid JSON: %v", err)
	}
	if _, ok := rawCheck["data_compliance"]; !ok {
		t.Errorf("RawData missing data_compliance — looks re-marshaled from typed struct")
	}
	if _, ok := rawCheck["company_name"]; !ok {
		t.Errorf("RawData missing company_name — looks re-marshaled from typed struct")
	}
	if _, ok := rawCheck["updated_at"]; !ok {
		t.Errorf("RawData missing updated_at — looks re-marshaled from typed struct")
	}
}

func TestGreenhouseAdapter_MissingOptionalFields(t *testing.T) {
	fixture := loadFixture(t, "jobs_missing_optional.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	p := postings[0]
	if p.LocationText != nil {
		t.Errorf("LocationText: got pointer to %q, want nil", *p.LocationText)
	}
	if p.Department != nil {
		t.Errorf("Department: got pointer to %q, want nil", *p.Department)
	}
	if p.PostedAt != nil {
		t.Errorf("PostedAt: got pointer to %v, want nil (Greenhouse adapter never sets PostedAt)", *p.PostedAt)
	}
	if p.SourceID != "9876543001" {
		t.Errorf("SourceID: got %q", p.SourceID)
	}
}

func TestGreenhouseAdapter_Non2xxReturnsError(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"unauthorized", http.StatusUnauthorized},
		{"not_found", http.StatusNotFound},
		{"internal_server_error", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "error", tc.status)
			}))
			t.Cleanup(srv.Close)

			postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
			if err == nil {
				t.Fatalf("FetchPostings: got nil error, want non-nil")
			}
			if postings != nil {
				t.Errorf("postings: got %v, want nil on error", postings)
			}
			statusStr := strconv.Itoa(tc.status)
			if !strings.Contains(err.Error(), statusStr) {
				t.Errorf("error %q does not mention status %s", err.Error(), statusStr)
			}
		})
	}
}

func TestGreenhouseAdapter_MalformedJSONReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs": [not valid json`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}

func TestGreenhouseAdapter_EmptyJobsList_ReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs": [], "meta": {"total": 0}}`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 0 {
		t.Errorf("got %d postings, want 0", len(postings))
	}
}

func TestGreenhouseAdapter_CancelledContext_ReturnsError(t *testing.T) {
	// Server that blocks until the test ends, so context cancellation is the only exit.
	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-unblock
	}))
	t.Cleanup(func() {
		close(unblock)
		srv.Close()
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel before the request starts

	postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(ctx, "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil after context cancel")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}

func TestGreenhouseAdapter_MultipleDepartments_UsesFirst(t *testing.T) {
	// Two departments: adapter must pick the first one, not the last or deepest.
	body := `{"jobs": [{"id": 1, "title": "Eng Role", "departments": [
		{"id": 10, "name": "Engineering"},
		{"id": 11, "name": "Platform"}
	]}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	if postings[0].Department == nil || *postings[0].Department != "Engineering" {
		t.Errorf("Department: got %v, want pointer to %q", postings[0].Department, "Engineering")
	}
}

func TestGreenhouseAdapter_ZeroID_ReturnsError(t *testing.T) {
	// id missing from the wire response decodes as 0; adapter must reject it
	// rather than emit SourceID="0" and a SourceURL ending in /jobs/0.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs": [{"title": "No ID Role"}]}`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for id==0")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}

func TestGreenhouseAdapter_MalformedJobEntry_ReturnsError(t *testing.T) {
	// Outer envelope is valid JSON; individual job object is not.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs": [{not valid json}]}`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for malformed job entry")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}
