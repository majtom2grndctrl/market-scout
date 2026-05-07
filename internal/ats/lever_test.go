package ats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLever_SuccessfulParse(t *testing.T) {
	fixture := loadAdapterFixture(t, "lever", "jobs_full.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/leverdemo"; got != want {
			t.Errorf("path: got %q, want %q", got, want)
		}
		q := r.URL.Query()
		if got := q.Get("limit"); got != strconv.Itoa(leverPageSize) {
			t.Errorf("limit query: got %q, want %q", got, strconv.Itoa(leverPageSize))
		}
		if got := q.Get("mode"); got != "json" {
			t.Errorf("mode query: got %q, want %q", got, "json")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 2 {
		t.Fatalf("got %d postings, want 2", len(postings))
	}

	p := postings[0]
	if p.SourceID != "d88d75e3-9465-4ea9-93d1-129e835c4cd9" {
		t.Errorf("SourceID: got %q", p.SourceID)
	}
	wantURL := "https://jobs.lever.co/leverdemo/d88d75e3-9465-4ea9-93d1-129e835c4cd9"
	if p.SourceURL != wantURL {
		t.Errorf("SourceURL: got %q, want %q", p.SourceURL, wantURL)
	}
	if p.JobURL == nil || *p.JobURL != wantURL {
		t.Errorf("JobURL: got %v, want pointer to %q", p.JobURL, wantURL)
	}
	if p.Title == nil || *p.Title != "Senior Designer" {
		t.Errorf("Title: got %v", p.Title)
	}
	if p.Team == nil || *p.Team != "Design Systems" {
		t.Errorf("Team: got %v", p.Team)
	}
	if p.Department == nil || *p.Department != "Design" {
		t.Errorf("Department: got %v", p.Department)
	}
	if p.WorkplaceType == nil || *p.WorkplaceType != "remote" {
		t.Errorf("WorkplaceType: got %v, want pointer to %q", p.WorkplaceType, "remote")
	}
	if p.EmploymentType == nil || *p.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %v, want pointer to %q", p.EmploymentType, "full_time")
	}
	wantTexts := []string{"Amsterdam, Netherlands", "Berlin, Germany", "Remote - EU"}
	if len(p.LocationTexts) != len(wantTexts) {
		t.Fatalf("LocationTexts len: got %d, want %d", len(p.LocationTexts), len(wantTexts))
	}
	for i, want := range wantTexts {
		if p.LocationTexts[i] != want {
			t.Errorf("LocationTexts[%d]: got %q, want %q", i, p.LocationTexts[i], want)
		}
	}
	if p.LocationText == nil || *p.LocationText != "Amsterdam, Netherlands" {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, "Amsterdam, Netherlands")
	}
	wantPostedAt := time.UnixMilli(1714521600000).UTC()
	if p.PostedAt == nil || !p.PostedAt.Equal(wantPostedAt) {
		t.Errorf("PostedAt: got %v, want %v", p.PostedAt, wantPostedAt)
	}
	if p.SourceFirstPublishedAt == nil || !p.SourceFirstPublishedAt.Equal(wantPostedAt) {
		t.Errorf("SourceFirstPublishedAt: got %v, want %v", p.SourceFirstPublishedAt, wantPostedAt)
	}
	if p.SourceLastModifiedAt != nil {
		t.Errorf("SourceLastModifiedAt: got %v, want nil (Lever exposes no last-modified)", *p.SourceLastModifiedAt)
	}

	// RawData should preserve fields not in the typed struct (e.g. additional, country).
	var rawCheck map[string]json.RawMessage
	if err := json.Unmarshal(p.RawData, &rawCheck); err != nil {
		t.Fatalf("RawData is not valid JSON: %v", err)
	}
	if _, ok := rawCheck["additional"]; !ok {
		t.Errorf("RawData missing additional — looks re-marshaled from typed struct")
	}
	if _, ok := rawCheck["country"]; !ok {
		t.Errorf("RawData missing country — looks re-marshaled from typed struct")
	}
}

func TestLever_PaginatesUntilShortPage(t *testing.T) {
	// Generate two pages: page 1 (skip=0) returns leverPageSize jobs, page 2
	// (skip=leverPageSize) returns 1 job, signaling termination.
	pageOne := make([]map[string]any, leverPageSize)
	for i := 0; i < leverPageSize; i++ {
		pageOne[i] = map[string]any{
			"id":        fmt.Sprintf("p1-%d", i),
			"text":      "Role",
			"hostedUrl": fmt.Sprintf("https://jobs.lever.co/leverdemo/p1-%d", i),
		}
	}
	pageTwo := []map[string]any{
		{"id": "p2-0", "text": "Last", "hostedUrl": "https://jobs.lever.co/leverdemo/p2-0"},
	}
	pageOneBytes, _ := json.Marshal(pageOne)
	pageTwoBytes, _ := json.Marshal(pageTwo)

	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		skip := r.URL.Query().Get("skip")
		switch skip {
		case "0":
			_, _ = w.Write(pageOneBytes)
		case strconv.Itoa(leverPageSize):
			_, _ = w.Write(pageTwoBytes)
		default:
			t.Errorf("unexpected skip value: %q", skip)
			http.Error(w, "unexpected skip", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	wantTotal := leverPageSize + 1
	if len(postings) != wantTotal {
		t.Fatalf("got %d postings, want %d", len(postings), wantTotal)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Errorf("request count: got %d, want 2", got)
	}
	if postings[0].SourceID != "p1-0" {
		t.Errorf("first posting SourceID: got %q, want %q", postings[0].SourceID, "p1-0")
	}
	if postings[len(postings)-1].SourceID != "p2-0" {
		t.Errorf("last posting SourceID: got %q, want %q", postings[len(postings)-1].SourceID, "p2-0")
	}
}

func TestLever_MissingOptionalFields(t *testing.T) {
	fixture := loadAdapterFixture(t, "lever", "jobs_missing_optional.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	p := postings[0]
	if p.Title != nil {
		t.Errorf("Title: got %v, want nil", *p.Title)
	}
	if p.LocationText != nil {
		t.Errorf("LocationText: got %v, want nil", *p.LocationText)
	}
	if p.LocationTexts != nil {
		t.Errorf("LocationTexts: got %v, want nil", p.LocationTexts)
	}
	if p.Team != nil {
		t.Errorf("Team: got %v, want nil", *p.Team)
	}
	if p.Department != nil {
		t.Errorf("Department: got %v, want nil", *p.Department)
	}
	if p.WorkplaceType != nil {
		t.Errorf("WorkplaceType: got %v, want nil", *p.WorkplaceType)
	}
	if p.EmploymentType != nil {
		t.Errorf("EmploymentType: got %v, want nil", *p.EmploymentType)
	}
	if p.PostedAt != nil {
		t.Errorf("PostedAt: got %v, want nil", *p.PostedAt)
	}
	if p.SourceFirstPublishedAt != nil {
		t.Errorf("SourceFirstPublishedAt: got %v, want nil", *p.SourceFirstPublishedAt)
	}
}

func TestLever_LocationFallbackToCategoriesLocation(t *testing.T) {
	fixture := loadAdapterFixture(t, "lever", "jobs_location_fallback.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	p := postings[0]
	if len(p.LocationTexts) != 1 || p.LocationTexts[0] != "Singapore" {
		t.Errorf("LocationTexts: got %v, want [%q]", p.LocationTexts, "Singapore")
	}
	if p.LocationText == nil || *p.LocationText != "Singapore" {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, "Singapore")
	}
}

func TestLever_WorkplaceTypeNormalization(t *testing.T) {
	cases := []struct {
		name string
		wire string
		want *string
	}{
		{"remote", "remote", strPtr("remote")},
		{"onsite", "onsite", strPtr("onsite")},
		{"hybrid", "hybrid", strPtr("hybrid")},
		{"unspecified_to_nil", "unspecified", nil},
		{"unrecognized_to_nil_with_warn", "spaceship", nil},
		{"empty_to_nil", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`[{"id":"x","hostedUrl":"https://example.com/x","workplaceType":%q}]`, tc.wire)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(srv.Close)

			postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
			if err != nil {
				t.Fatalf("FetchPostings: %v", err)
			}
			if len(postings) != 1 {
				t.Fatalf("got %d postings, want 1", len(postings))
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

func TestLever_EmploymentTypeNormalization(t *testing.T) {
	cases := []struct {
		name string
		wire string
		want *string
	}{
		{"full_time_titlecase", "Full-time", strPtr("full_time")},
		{"full_time_lower_with_space", "full time", strPtr("full_time")},
		{"full_time_abbrev", "FT", strPtr("full_time")},
		{"part_time_lower", "part time", strPtr("part_time")},
		{"contract", "Contract", strPtr("contract")},
		{"intern", "Internship", strPtr("intern")},
		{"unknown_to_nil_with_warn", "Volunteer", nil},
		{"empty_to_nil", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`[{"id":"x","hostedUrl":"https://example.com/x","categories":{"commitment":%q}}]`, tc.wire)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(srv.Close)

			postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
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

func TestLever_PostedAtFromCreatedAt(t *testing.T) {
	body := `[{"id":"x","hostedUrl":"https://example.com/x","createdAt":1714521600000}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	want := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	if postings[0].PostedAt == nil || !postings[0].PostedAt.Equal(want) {
		t.Errorf("PostedAt: got %v, want %v", postings[0].PostedAt, want)
	}
}

func TestLever_Non2xxReturnsError(t *testing.T) {
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

			postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
			if err == nil {
				t.Fatalf("FetchPostings: got nil error, want non-nil")
			}
			if postings != nil {
				t.Errorf("postings: got %v, want nil", postings)
			}
			if !strings.Contains(err.Error(), "lever:") {
				t.Errorf("error %q missing lever: prefix", err.Error())
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

func TestLever_MalformedJSONReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[not valid json`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil", postings)
	}
	if !strings.Contains(err.Error(), "lever:") {
		t.Errorf("error %q missing lever: prefix", err.Error())
	}
}

func TestLever_MissingIDReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"hostedUrl":"https://example.com/x"}]`))
	}))
	t.Cleanup(srv.Close)

	_, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for missing id")
	}
	if !strings.Contains(err.Error(), "lever:") {
		t.Errorf("error %q missing lever: prefix", err.Error())
	}
}

func TestLever_MissingHostedURLReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"abc"}]`))
	}))
	t.Cleanup(srv.Close)

	_, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for missing hostedUrl")
	}
}

func TestLever_OversizeResponseWrappedWithLeverPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 32 MiB allocation in -short mode")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, maxResponseBytes+1))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want oversize error")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil", postings)
	}
	msg := err.Error()
	if !strings.Contains(msg, "lever:") {
		t.Errorf("error %q missing lever: prefix", msg)
	}
	if !strings.Contains(msg, "httpfetch:") {
		t.Errorf("error %q does not wrap httpfetch: helper error", msg)
	}
}

// TestLever_MissingIDInMidFetch_AbortsWithoutPartial exercises the AC that a
// missing `id` on any job aborts the company's fetch and returns a wrapped
// lever: error with no partial slice.
func TestLever_MissingIDInMidFetch_AbortsWithoutPartial(t *testing.T) {
	// Two jobs: first has a valid id, second has an empty id. The adapter must
	// abort after the second job and return nil postings.
	body := `[` +
		`{"id":"valid-1","hostedUrl":"https://jobs.lever.co/leverdemo/valid-1","text":"First"},` +
		`{"id":"","hostedUrl":"https://jobs.lever.co/leverdemo/missing-id","text":"Second"}` +
		`]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want error for missing id")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil (no partial results)", postings)
	}
	if !strings.Contains(err.Error(), "lever:") {
		t.Errorf("error %q missing lever: prefix", err.Error())
	}
}

// TestLever_AllLocationsExplicitEmpty_ProducesEmptyNonNilSlice pins the
// nil-vs-empty distinction for LocationTexts: when the wire delivers
// `categories.allLocations: []` (explicit empty array), the adapter must
// emit a non-nil empty slice, not fall back to `categories.location`. A
// missing `allLocations` field is covered separately by the location
// fallback test.
func TestLever_AllLocationsExplicitEmpty_ProducesEmptyNonNilSlice(t *testing.T) {
	fixture := loadAdapterFixture(t, "lever", "jobs_all_locations_empty.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	p := postings[0]
	if p.LocationTexts == nil {
		t.Fatalf("LocationTexts: got nil, want non-nil empty slice")
	}
	if len(p.LocationTexts) != 0 {
		t.Errorf("LocationTexts: got %v, want empty slice", p.LocationTexts)
	}
	if p.LocationText != nil {
		t.Errorf("LocationText: got %q, want nil (no first element)", *p.LocationText)
	}
}

// TestLever_PaginationExactlyFullPage covers the boundary where the first
// page returns exactly leverPageSize jobs and the second page returns an
// empty array. The loop must issue the second request (since the first is
// not "short") and terminate cleanly when the empty page arrives.
func TestLever_PaginationExactlyFullPage(t *testing.T) {
	pageOne := make([]map[string]any, leverPageSize)
	for i := 0; i < leverPageSize; i++ {
		pageOne[i] = map[string]any{
			"id":        fmt.Sprintf("p1-%d", i),
			"text":      "Role",
			"hostedUrl": fmt.Sprintf("https://jobs.lever.co/leverdemo/p1-%d", i),
		}
	}
	pageOneBytes, _ := json.Marshal(pageOne)
	emptyBytes := []byte(`[]`)

	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		switch r.URL.Query().Get("skip") {
		case "0":
			_, _ = w.Write(pageOneBytes)
		case strconv.Itoa(leverPageSize):
			_, _ = w.Write(emptyBytes)
		default:
			t.Errorf("unexpected skip value: %q", r.URL.Query().Get("skip"))
			http.Error(w, "unexpected skip", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != leverPageSize {
		t.Fatalf("got %d postings, want %d", len(postings), leverPageSize)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Errorf("request count: got %d, want 2", got)
	}
}

// TestLever_PaginationEmptyFirstPage covers the zero-jobs case: the first
// request returns an empty array, the loop terminates immediately, and the
// adapter returns nil postings without error.
func TestLever_PaginationEmptyFirstPage(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil", postings)
	}
	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Errorf("request count: got %d, want 1", got)
	}
}

// TestLeverAdapter_PrefersDescriptionPlain pins the precedence rule: when both
// descriptionPlain and description are present, the adapter uses the wire's
// pre-flattened plain text rather than running its own HTML→text conversion.
// This avoids round-tripping content through the local sanitizer when the
// upstream already provided a clean version.
func TestLeverAdapter_PrefersDescriptionPlain(t *testing.T) {
	body := `[{"id":"x","hostedUrl":"https://example.com/x","descriptionPlain":"Plain text","description":"<p>HTML version</p>"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	p := postings[0]
	if p.DescriptionText == nil {
		t.Fatalf("DescriptionText: got nil, want pointer to %q", "Plain text")
	}
	if got, want := *p.DescriptionText, "Plain text"; got != want {
		t.Errorf("DescriptionText: got %q, want %q (must not run HTML→text on `description` when `descriptionPlain` is present)", got, want)
	}
}

// TestLeverAdapter_StripsHTMLWhenNoDescriptionPlain verifies the fallback path:
// when descriptionPlain is empty/absent, the adapter renders the HTML
// `description` field through htmlToPlainText.
func TestLeverAdapter_StripsHTMLWhenNoDescriptionPlain(t *testing.T) {
	body := `[{"id":"x","hostedUrl":"https://example.com/x","descriptionPlain":"","description":"<p>HTML</p>"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	p := postings[0]
	if p.DescriptionText == nil {
		t.Fatalf("DescriptionText: got nil, want pointer to %q", "HTML")
	}
	if got, want := *p.DescriptionText, "HTML"; got != want {
		t.Errorf("DescriptionText: got %q, want %q", got, want)
	}
}

// TestLeverAdapter_PopulatesCompensation pins the salaryRange → schema mapping:
// well-formed wire data produces all four comp fields with the interval mapped
// through leverIntervalAliases ("per-year-salary" → "year").
func TestLeverAdapter_PopulatesCompensation(t *testing.T) {
	body := `[{"id":"x","hostedUrl":"https://example.com/x","salaryRange":{"min":150000,"max":200000,"currency":"USD","interval":"per-year-salary"}}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	p := postings[0]
	if p.CompensationMin == nil || *p.CompensationMin != 150000 {
		t.Errorf("CompensationMin: got %v, want pointer to 150000", p.CompensationMin)
	}
	if p.CompensationMax == nil || *p.CompensationMax != 200000 {
		t.Errorf("CompensationMax: got %v, want pointer to 200000", p.CompensationMax)
	}
	if p.CompensationCurrency == nil || *p.CompensationCurrency != "USD" {
		t.Errorf("CompensationCurrency: got %v, want pointer to %q", p.CompensationCurrency, "USD")
	}
	if p.CompensationPeriod == nil || *p.CompensationPeriod != "year" {
		t.Errorf("CompensationPeriod: got %v, want pointer to %q", p.CompensationPeriod, "year")
	}
}

// TestLeverAdapter_NilCompOnUnknownInterval pins the all-or-nothing rule on the
// comp fields: an unrecognized interval drops every field, even though min/max
// and currency are otherwise well-formed. Partial population is never allowed.
func TestLeverAdapter_NilCompOnUnknownInterval(t *testing.T) {
	body := `[{"id":"x","hostedUrl":"https://example.com/x","salaryRange":{"min":1000,"max":2000,"currency":"USD","interval":"per-fortnight"}}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newLeverWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "leverdemo")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	p := postings[0]
	if p.CompensationMin != nil {
		t.Errorf("CompensationMin: got %v, want nil", *p.CompensationMin)
	}
	if p.CompensationMax != nil {
		t.Errorf("CompensationMax: got %v, want nil", *p.CompensationMax)
	}
	if p.CompensationCurrency != nil {
		t.Errorf("CompensationCurrency: got %v, want nil", *p.CompensationCurrency)
	}
	if p.CompensationPeriod != nil {
		t.Errorf("CompensationPeriod: got %v, want nil", *p.CompensationPeriod)
	}
}

func strPtr(s string) *string { return &s }
