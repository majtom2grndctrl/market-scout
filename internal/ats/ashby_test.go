package ats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAshby_SuccessfulParse(t *testing.T) {
	fixture := loadAdapterFixture(t, "ashby", "jobs_full.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/example"; got != want {
			t.Errorf("path: got %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 2 {
		t.Fatalf("got %d postings, want 2", len(postings))
	}

	p := postings[0]
	if p.SourceID != "7458d4e9-da2e-47bd-98cb-adfda43d42b2" {
		t.Errorf("SourceID: got %q", p.SourceID)
	}
	wantURL := "https://jobs.ashbyhq.com/example/7458d4e9-da2e-47bd-98cb-adfda43d42b2"
	if p.SourceURL != wantURL {
		t.Errorf("SourceURL: got %q, want %q", p.SourceURL, wantURL)
	}
	if p.JobURL == nil || *p.JobURL != wantURL {
		t.Errorf("JobURL: got %v, want pointer to %q", p.JobURL, wantURL)
	}
	if p.Title == nil || *p.Title != "Engineering Manager, EU" {
		t.Errorf("Title: got %v", p.Title)
	}
	if p.Department == nil || *p.Department != "Engineering" {
		t.Errorf("Department: got %v", p.Department)
	}
	if p.Team == nil || *p.Team != "EMEA Engineering" {
		t.Errorf("Team: got %v", p.Team)
	}
	if p.WorkplaceType == nil || *p.WorkplaceType != "remote" {
		t.Errorf("WorkplaceType: got %v, want pointer to %q", p.WorkplaceType, "remote")
	}
	if p.EmploymentType == nil || *p.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %v, want pointer to %q", p.EmploymentType, "full_time")
	}
	// Both secondaries have populated postal addresses AND distinct `location`
	// strings ("Bucharest Office", "BCN HQ"). Asserting on the postal renders
	// — not the location strings — pins postal-takes-precedence in the
	// renderAshbyLocations resolution rule.
	wantTexts := []string{
		"Remote - European Union",
		"Romania",                     // postal: locality+region empty, country only
		"Barcelona, Catalonia, Spain", // postal: all three populated
	}
	if len(p.LocationTexts) != len(wantTexts) {
		t.Fatalf("LocationTexts len: got %d (%v), want %d", len(p.LocationTexts), p.LocationTexts, len(wantTexts))
	}
	for i, want := range wantTexts {
		if p.LocationTexts[i] != want {
			t.Errorf("LocationTexts[%d]: got %q, want %q", i, p.LocationTexts[i], want)
		}
	}
	if p.LocationText == nil || *p.LocationText != "Remote - European Union" {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, "Remote - European Union")
	}
	wantPosted := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	if p.PostedAt == nil || !p.PostedAt.Equal(wantPosted) {
		t.Errorf("PostedAt: got %v, want %v", p.PostedAt, wantPosted)
	}
	if p.SourceFirstPublishedAt == nil || !p.SourceFirstPublishedAt.Equal(wantPosted) {
		t.Errorf("SourceFirstPublishedAt: got %v, want %v", p.SourceFirstPublishedAt, wantPosted)
	}
	if p.SourceLastModifiedAt != nil {
		t.Errorf("SourceLastModifiedAt: got %v, want nil (Ashby exposes no last-modified)", *p.SourceLastModifiedAt)
	}

	// RFC3339Nano with milliseconds + offset (job 2)
	p2 := postings[1]
	wantPosted2 := time.Date(2026, 3, 1, 9, 30, 0, 123_000_000, time.UTC)
	if p2.PostedAt == nil || !p2.PostedAt.Equal(wantPosted2) {
		t.Errorf("posting 2 PostedAt: got %v, want %v", p2.PostedAt, wantPosted2)
	}
	if p2.WorkplaceType == nil || *p2.WorkplaceType != "onsite" {
		t.Errorf("posting 2 WorkplaceType: got %v, want pointer to %q", p2.WorkplaceType, "onsite")
	}
	if p2.EmploymentType == nil || *p2.EmploymentType != "contract" {
		t.Errorf("posting 2 EmploymentType: got %v, want pointer to %q", p2.EmploymentType, "contract")
	}

	// RawData should preserve the per-job bytes verbatim.
	var rawCheck map[string]json.RawMessage
	if err := json.Unmarshal(p.RawData, &rawCheck); err != nil {
		t.Fatalf("RawData is not valid JSON: %v", err)
	}
	if _, ok := rawCheck["secondaryLocations"]; !ok {
		t.Errorf("RawData missing secondaryLocations — looks re-marshaled from typed struct")
	}
}

func TestAshby_WorkplaceTypeNormalization(t *testing.T) {
	cases := []struct {
		name string
		wire string
		want *string
	}{
		{"remote", "Remote", strPtr("remote")},
		{"onsite", "OnSite", strPtr("onsite")},
		{"hybrid", "Hybrid", strPtr("hybrid")},
		{"unknown_to_nil_with_warn", "Spaceship", nil},
		{"empty_to_nil", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"jobs":[{"id":"x","jobUrl":"https://example.com/x","workplaceType":%q}]}`, tc.wire)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(srv.Close)

			postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
			if err != nil {
				t.Fatalf("FetchPostings: %v", err)
			}
			got := postings[0].WorkplaceType
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("WorkplaceType: got %q, want nil", *got)
			case tc.want != nil && got == nil:
				t.Errorf("WorkplaceType: got nil, want %q", *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Errorf("WorkplaceType: got %q, want %q", *got, *tc.want)
			}
		})
	}
}

func TestAshby_EmploymentTypeNormalization(t *testing.T) {
	cases := []struct {
		name string
		wire string
		want *string
	}{
		{"FullTime", "FullTime", strPtr("full_time")},
		{"PartTime", "PartTime", strPtr("part_time")},
		{"Contract", "Contract", strPtr("contract")},
		{"Intern", "Intern", strPtr("intern")},
		{"Temporary", "Temporary", strPtr("temporary")},
		{"unknown_to_nil_with_warn", "Sabbatical", nil},
		{"empty_to_nil", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"jobs":[{"id":"x","jobUrl":"https://example.com/x","employmentType":%q}]}`, tc.wire)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(srv.Close)

			postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
			if err != nil {
				t.Fatalf("FetchPostings: %v", err)
			}
			got := postings[0].EmploymentType
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("EmploymentType: got %q, want nil", *got)
			case tc.want != nil && got == nil:
				t.Errorf("EmploymentType: got nil, want %q", *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Errorf("EmploymentType: got %q, want %q", *got, *tc.want)
			}
		})
	}
}

func TestAshby_PublishedAtMissing_NilPostedAt(t *testing.T) {
	body := `{"jobs":[{"id":"x","jobUrl":"https://example.com/x"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if postings[0].PostedAt != nil {
		t.Errorf("PostedAt: got %v, want nil", *postings[0].PostedAt)
	}
}

func TestAshby_MissingIDReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs":[{"jobUrl":"https://example.com/x"}]}`))
	}))
	t.Cleanup(srv.Close)

	_, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for missing id")
	}
	if !strings.Contains(err.Error(), "ashby:") {
		t.Errorf("error %q missing ashby: prefix", err.Error())
	}
}

func TestAshby_MissingJobURLReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs":[{"id":"abc"}]}`))
	}))
	t.Cleanup(srv.Close)

	_, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for missing jobUrl")
	}
	if !strings.Contains(err.Error(), "ashby:") {
		t.Errorf("error %q missing ashby: prefix", err.Error())
	}
}

func TestAshby_Non2xxReturnsError(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"not_found", http.StatusNotFound},
		{"internal_server_error", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "boom", tc.status)
			}))
			t.Cleanup(srv.Close)

			postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
			if err == nil {
				t.Fatalf("FetchPostings: got nil error, want non-nil")
			}
			if postings != nil {
				t.Errorf("postings: got %v, want nil", postings)
			}
			if !strings.Contains(err.Error(), "ashby:") {
				t.Errorf("error %q missing ashby: prefix", err.Error())
			}
			if !strings.Contains(err.Error(), "httpfetch:") {
				t.Errorf("error %q does not wrap httpfetch: helper error", err.Error())
			}
			if !strings.Contains(err.Error(), strconv.Itoa(tc.status)) {
				t.Errorf("error %q does not include status %d", err.Error(), tc.status)
			}
		})
	}
}

func TestAshby_MalformedJSONReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs": [not valid`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil", postings)
	}
	if !strings.Contains(err.Error(), "ashby:") {
		t.Errorf("error %q missing ashby: prefix", err.Error())
	}
}

func TestAshby_OversizeResponseWrappedWithAshbyPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("oversize-response test allocates ~32 MiB; skipped under -short")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, maxResponseBytes+1))
	}))
	t.Cleanup(srv.Close)

	postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want oversize error")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil", postings)
	}
	msg := err.Error()
	if !strings.Contains(msg, "ashby:") {
		t.Errorf("error %q missing ashby: prefix", msg)
	}
	if !strings.Contains(msg, "httpfetch:") {
		t.Errorf("error %q does not wrap httpfetch: helper error", msg)
	}
}

// TestAshby_MissingIDInMidFetch_AbortsWithoutPartial verifies that a missing id
// on a job that is not the first in the list aborts the fetch and returns no
// partial slice. The valid job that was already decoded must not be returned.
func TestAshby_MissingIDInMidFetch_AbortsWithoutPartial(t *testing.T) {
	body := `{"jobs":[` +
		`{"id":"valid-job-1","jobUrl":"https://jobs.ashbyhq.com/example/valid-job-1","title":"First Job"},` +
		`{"jobUrl":"https://jobs.ashbyhq.com/example/missing-id","title":"Second Job"}` +
		`]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for missing id")
	}
	if postings != nil {
		t.Errorf("postings: got %v (len %d), want nil — no partial results on error", postings, len(postings))
	}
	if !strings.Contains(err.Error(), "ashby:") {
		t.Errorf("error %q missing ashby: prefix", err.Error())
	}
}

// TestAshby_SecondaryLocationResolution pins the resolution rule documented on
// renderAshbyLocations: postal address wins when non-empty; the secondary's
// `location` string is the fallback when postal is empty; both empty drops the
// entry. Recorded fixture (not hand-written) keeps the wire shape honest.
func TestAshby_SecondaryLocationResolution(t *testing.T) {
	fixture := loadAdapterFixture(t, "ashby", "jobs_secondary_location_fallback.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}

	// Expected resolution:
	//   primary           -> "Remote - Global"
	//   secondary[0]      -> postal empty, location "Tokyo Office" -> fallback
	//   secondary[1]      -> postal populated -> postal wins ("Berlin HQ" ignored)
	//   secondary[2]      -> postal empty, location empty -> dropped
	wantTexts := []string{
		"Remote - Global",
		"Tokyo Office",
		"Berlin, Berlin, Germany",
	}
	got := postings[0].LocationTexts
	if len(got) != len(wantTexts) {
		t.Fatalf("LocationTexts len: got %d (%v), want %d (%v)", len(got), got, len(wantTexts), wantTexts)
	}
	for i, want := range wantTexts {
		if got[i] != want {
			t.Errorf("LocationTexts[%d]: got %q, want %q", i, got[i], want)
		}
	}
	for _, text := range got {
		if text == "" {
			t.Errorf("LocationTexts contains empty entry: %v", got)
		}
		if text == "Berlin HQ" {
			t.Errorf("LocationTexts[i] = %q — postal-takes-precedence violated; secondary's `location` was used despite a populated postal address", text)
		}
	}
}

// TestAshby_EmptyPostalAddressParts_AreSkipped verifies that secondary locations
// where all three postal-address parts (addressLocality, addressRegion,
// addressCountry) are empty strings render to nothing and are excluded from
// LocationTexts — no ", , " artifacts appear and no empty entry is added.
func TestAshby_EmptyPostalAddressParts_AreSkipped(t *testing.T) {
	body := `{"jobs":[{"id":"x","jobUrl":"https://jobs.ashbyhq.com/example/x",` +
		`"location":"Remote",` +
		`"secondaryLocations":[` +
		`{"location":"","address":{"postalAddress":{"addressLocality":"","addressRegion":"","addressCountry":""}}}` +
		`]}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newAshbyWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "example")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	texts := postings[0].LocationTexts
	// Only the primary "Remote" should appear; the all-empty secondary must be dropped.
	if len(texts) != 1 {
		t.Fatalf("LocationTexts len: got %d (%v), want 1", len(texts), texts)
	}
	if texts[0] != "Remote" {
		t.Errorf("LocationTexts[0]: got %q, want %q", texts[0], "Remote")
	}
	for _, text := range texts {
		if strings.Contains(text, ", ,") || strings.Contains(text, ",,") {
			t.Errorf("LocationTexts contains artifact %q", text)
		}
	}
}
