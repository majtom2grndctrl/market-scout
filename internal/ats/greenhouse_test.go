package ats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withGreenhouseBaseURL points the package-level base URL at the test server
// for the duration of the test, restoring the original value via t.Cleanup.
func withGreenhouseBaseURL(t *testing.T, url string) {
	t.Helper()
	prev := greenhouseBaseURL
	greenhouseBaseURL = url
	t.Cleanup(func() { greenhouseBaseURL = prev })
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "greenhouse", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
}

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

	withGreenhouseBaseURL(t, srv.URL)

	adapter := New(nil)
	postings, err := adapter.FetchPostings(context.Background(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	p := postings[0]

	if p.SourceID != "4567890" {
		t.Errorf("SourceID: got %q, want %q", p.SourceID, "4567890")
	}
	wantSourceURL := srv.URL + "/exampleco/jobs/4567890"
	if p.SourceURL != wantSourceURL {
		t.Errorf("SourceURL: got %q, want %q", p.SourceURL, wantSourceURL)
	}
	if p.Title != "Senior Software Engineer, Platform" {
		t.Errorf("Title: got %q", p.Title)
	}
	if p.LocationText == nil || *p.LocationText != "San Francisco, CA" {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, "San Francisco, CA")
	}
	if p.Department == nil || *p.Department != "Engineering" {
		t.Errorf("Department: got %v, want pointer to %q", p.Department, "Engineering")
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
		t.Errorf("PostedAt: got %v, want nil", *p.PostedAt)
	}
	if p.JobURL != "https://boards.greenhouse.io/exampleco/jobs/4567890" {
		t.Errorf("JobURL: got %q", p.JobURL)
	}

	// RawData should be the original raw bytes — verify a field that the typed
	// wire struct does NOT declare survives (e.g., requisition_id, metadata).
	var rawCheck map[string]json.RawMessage
	if err := json.Unmarshal(p.RawData, &rawCheck); err != nil {
		t.Fatalf("RawData is not valid JSON: %v", err)
	}
	if _, ok := rawCheck["requisition_id"]; !ok {
		t.Errorf("RawData missing requisition_id — looks re-marshaled from typed struct")
	}
	if _, ok := rawCheck["metadata"]; !ok {
		t.Errorf("RawData missing metadata — looks re-marshaled from typed struct")
	}
}

func TestGreenhouseAdapter_MissingOptionalFields(t *testing.T) {
	fixture := loadFixture(t, "jobs_missing_optional.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	withGreenhouseBaseURL(t, srv.URL)

	postings, err := New(nil).FetchPostings(context.Background(), "exampleco")
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
	if p.SourceID != "9876543" {
		t.Errorf("SourceID: got %q", p.SourceID)
	}
}

func TestGreenhouseAdapter_Non2xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	withGreenhouseBaseURL(t, srv.URL)

	postings, err := New(nil).FetchPostings(context.Background(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status 500", err.Error())
	}
}

func TestGreenhouseAdapter_MalformedJSONReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs": [not valid json`))
	}))
	t.Cleanup(srv.Close)

	withGreenhouseBaseURL(t, srv.URL)

	postings, err := New(nil).FetchPostings(context.Background(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}
