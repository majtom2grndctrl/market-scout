package ats

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	workableBoardToken = "acme-robotics"
	workableWantPath   = "/acme-robotics"
)

func TestWorkableAdapter_SuccessfulParse(t *testing.T) {
	fixture := loadAdapterFixture(t, "workable", "jobs_full.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %q, want GET", r.Method)
		}
		if r.URL.Path != workableWantPath {
			t.Errorf("path: got %q, want %q", r.URL.Path, workableWantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkableWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workableBoardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}

	p := postings[0]
	if p.SourceID != "66FF2331A3" {
		t.Errorf("SourceID: got %q, want %q", p.SourceID, "66FF2331A3")
	}
	wantURL := "https://apply.workable.com/j/66FF2331A3"
	if p.SourceURL != wantURL {
		t.Errorf("SourceURL: got %q, want %q", p.SourceURL, wantURL)
	}
	if p.JobURL == nil || *p.JobURL != wantURL {
		t.Errorf("JobURL: got %v, want pointer to %q", p.JobURL, wantURL)
	}
	if p.Title == nil || *p.Title != "Analytics Engineer" {
		t.Errorf("Title: got %v, want pointer to %q", p.Title, "Analytics Engineer")
	}
	if p.Department == nil || *p.Department != "Analytics Engineering" {
		t.Errorf("Department: got %v, want pointer to %q", p.Department, "Analytics Engineering")
	}
	if p.EmploymentType == nil || *p.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %v, want pointer to %q", p.EmploymentType, "full_time")
	}
	// Flat city/state are empty on this real-data fixture; only country populates.
	wantLoc := "United States"
	if p.LocationText == nil || *p.LocationText != wantLoc {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, wantLoc)
	}
	if len(p.LocationTexts) != 1 || p.LocationTexts[0] != wantLoc {
		t.Errorf("LocationTexts: got %v, want [%q]", p.LocationTexts, wantLoc)
	}
	wantPostedAt := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	if p.PostedAt == nil || !p.PostedAt.Equal(wantPostedAt) {
		t.Errorf("PostedAt: got %v, want %v", p.PostedAt, wantPostedAt)
	}
	// Real Workable widget responses return created_at as a YYYY-MM-DD date,
	// not RFC3339. Adapter parses both forms.
	wantCreatedAt := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	if p.SourceFirstPublishedAt == nil || !p.SourceFirstPublishedAt.Equal(wantCreatedAt) {
		t.Errorf("SourceFirstPublishedAt: got %v, want %v", p.SourceFirstPublishedAt, wantCreatedAt)
	}
	if p.DescriptionText != nil {
		t.Errorf("DescriptionText: got %v, want nil (v1 omits descriptions)", *p.DescriptionText)
	}
	if p.CompensationMin != nil {
		t.Errorf("CompensationMin: got %v, want nil", *p.CompensationMin)
	}
	if p.SourceLastModifiedAt != nil {
		t.Errorf("SourceLastModifiedAt: got %v, want nil", *p.SourceLastModifiedAt)
	}
	if len(p.RawData) == 0 {
		t.Errorf("RawData: empty, want per-job raw bytes")
	}
}

func TestWorkableAdapter_MissingOptionalFields(t *testing.T) {
	fixture := loadAdapterFixture(t, "workable", "jobs_missing_optional.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkableWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workableBoardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	p := postings[0]
	if p.SourceID != "XYZ789" {
		t.Errorf("SourceID: got %q, want %q", p.SourceID, "XYZ789")
	}
	if p.LocationText != nil {
		t.Errorf("LocationText: got %v, want nil", *p.LocationText)
	}
	if p.LocationTexts != nil {
		t.Errorf("LocationTexts: got %v, want nil", p.LocationTexts)
	}
	if p.Department != nil {
		t.Errorf("Department: got %v, want nil", *p.Department)
	}
	if p.PostedAt != nil {
		t.Errorf("PostedAt: got %v, want nil", *p.PostedAt)
	}
	if p.SourceFirstPublishedAt != nil {
		t.Errorf("SourceFirstPublishedAt: got %v, want nil", *p.SourceFirstPublishedAt)
	}
}

func TestWorkableAdapter_EmptyBoard(t *testing.T) {
	fixture := loadAdapterFixture(t, "workable", "jobs_empty.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkableWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workableBoardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 0 {
		t.Errorf("got %d postings, want 0", len(postings))
	}
}

func TestWorkableAdapter_ApplicationURLFallback(t *testing.T) {
	fixture := loadAdapterFixture(t, "workable", "jobs_application_url_fallback.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkableWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workableBoardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	wantURL := "https://apply.workable.com/acme-robotics/j/CS001/apply/"
	if postings[0].SourceURL != wantURL {
		t.Errorf("SourceURL: got %q, want %q (fallback)", postings[0].SourceURL, wantURL)
	}
	if postings[0].JobURL == nil || *postings[0].JobURL != wantURL {
		t.Errorf("JobURL: got %v, want pointer to %q", postings[0].JobURL, wantURL)
	}
}

func TestWorkableAdapter_InvalidBoardToken(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"contains slash", "acme/extra"},
		{"contains scheme", "https://acme"},
		{"contains query", "acme?foo=bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requestCount int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&requestCount, 1)
				_, _ = w.Write([]byte(`{"jobs":[]}`))
			}))
			t.Cleanup(srv.Close)

			postings, err := newWorkableWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), tc.token)
			if err == nil {
				t.Fatalf("FetchPostings: got nil error, want non-nil for token %q", tc.token)
			}
			if postings != nil {
				t.Errorf("postings: got %v, want nil", postings)
			}
			if got := atomic.LoadInt32(&requestCount); got != 0 {
				t.Errorf("request count: got %d, want 0 (no HTTP request should be made)", got)
			}
		})
	}
}

func TestWorkableAdapter_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkableWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workableBoardToken)
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for HTTP 500")
	}
	if !strings.Contains(err.Error(), "workable:") {
		t.Errorf("error %q does not contain expected prefix %q", err.Error(), "workable:")
	}
	if len(postings) != 0 {
		t.Errorf("postings: got %d, want 0", len(postings))
	}
}

func TestWorkableAdapter_MissingShortcode(t *testing.T) {
	fixture := loadAdapterFixture(t, "workable", "jobs_missing_shortcode.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	_, err := newWorkableWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workableBoardToken)
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for missing shortcode")
	}
	if !strings.Contains(err.Error(), "index 0") {
		t.Errorf("error %q does not identify offending job (expected substring %q)", err.Error(), "index 0")
	}
	if !strings.Contains(err.Error(), "shortcode") {
		t.Errorf("error %q does not mention shortcode", err.Error())
	}
}
