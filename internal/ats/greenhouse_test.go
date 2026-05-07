package ats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGreenhouseAdapter_SuccessfulParse(t *testing.T) {
	fixture := loadAdapterFixture(t, "greenhouse", "jobs_full.json")

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

	adapter := newGreenhouseWithBaseURL(srv.Client(), srv.URL)
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
	// trailing spaces are preserved verbatim from the fixture — not a typo
	wantTitle := "Accenture Account Executive, Strategic (London, United Kingdom)  "
	if p.Title == nil || *p.Title != wantTitle {
		t.Errorf("Title: got %v, want pointer to %q", p.Title, wantTitle)
	}
	if p.LocationText == nil || *p.LocationText != "London, England" {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, "London, England")
	}
	// LocationTexts is a single-element array mirroring location.name — this is
	// the multi-source parity invariant called out in the plan.
	if len(p.LocationTexts) != 1 || p.LocationTexts[0] != "London, England" {
		t.Errorf("LocationTexts: got %v, want [%q]", p.LocationTexts, "London, England")
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
	fixture := loadAdapterFixture(t, "greenhouse", "jobs_missing_optional.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
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
	// nil (not empty slice) when location.name is absent — preserves the
	// nil-vs-empty distinction documented on domain.Posting.LocationTexts.
	if p.LocationTexts != nil {
		t.Errorf("LocationTexts: got %v, want nil", p.LocationTexts)
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

			postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
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

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
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

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
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

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(ctx, "exampleco")
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

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
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

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
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

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for malformed job entry")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}

func TestGreenhouseAdapter_ExplicitZeroID_ReturnsError(t *testing.T) {
	// id present in the wire response but set to 0: adapter must still reject it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jobs": [{"id": 0, "title": "Zero ID Role"}]}`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for explicit id==0")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}

func TestGreenhouseAdapter_BoardTokenURLEscaping(t *testing.T) {
	// boardToken containing characters that require URL path escaping.
	// url.PathEscape encodes '/' as '%2F' and ' ' as '%20'.
	boardToken := "example/co board"
	escapedToken := "example%2Fco%20board"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path is decoded by net/http; RawPath preserves the wire encoding.
		rawPath := r.URL.RawPath
		if rawPath == "" {
			rawPath = r.URL.Path
		}
		if !strings.Contains(rawPath, escapedToken) {
			t.Errorf("request path %q does not contain escaped token %q", rawPath, escapedToken)
		}
		_, _ = w.Write([]byte(`{"jobs": [{"id": 1234, "title": "Role"}]}`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), boardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	if !strings.Contains(postings[0].SourceURL, escapedToken) {
		t.Errorf("SourceURL %q does not contain escaped token %q", postings[0].SourceURL, escapedToken)
	}
}

func TestGreenhouseAdapter_SourceTimestamps_BothPresent(t *testing.T) {
	// Each case supplies a single job with both timestamps set and asserts the
	// parsed instants. want values are constructed with time.Date so the test
	// verifies the adapter produces the right instant, not merely that two parsers
	// agree. See testing-guide §4 (seam-crossing tests).
	cases := []struct {
		name           string
		firstPublished string
		updatedAt      string
		wantFirst      time.Time
		wantUpdated    time.Time
	}{
		{
			// numeric UTC offset, no fractional seconds
			name:           "numeric_offset_no_fraction",
			firstPublished: "2026-04-17T12:21:54-04:00",
			updatedAt:      "2026-04-17T16:21:54-04:00",
			wantFirst:      time.Date(2026, 4, 17, 12, 21, 54, 0, time.FixedZone("", -4*60*60)),
			wantUpdated:    time.Date(2026, 4, 17, 16, 21, 54, 0, time.FixedZone("", -4*60*60)),
		},
		{
			// Z suffix, nanosecond precision
			name:           "z_suffix_nanoseconds",
			firstPublished: "2026-04-18T09:30:00.123456789Z",
			updatedAt:      "2026-04-18T09:30:00.123456789Z",
			wantFirst:      time.Date(2026, 4, 18, 9, 30, 0, 123456789, time.UTC),
			wantUpdated:    time.Date(2026, 4, 18, 9, 30, 0, 123456789, time.UTC),
		},
		{
			// fractional seconds combined with a numeric UTC offset — Greenhouse can
			// emit this shape (e.g. 2024-08-12T17:33:21.456-04:00); previously
			// untested combination.
			name:           "fractional_seconds_and_numeric_offset",
			firstPublished: "2024-08-12T17:33:21.456-04:00",
			updatedAt:      "2024-08-12T21:33:21.456Z",
			wantFirst:      time.Date(2024, 8, 12, 17, 33, 21, 456_000_000, time.FixedZone("", -4*60*60)),
			wantUpdated:    time.Date(2024, 8, 12, 21, 33, 21, 456_000_000, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"jobs": [{"id": 42, "title": "Role", "first_published": "` +
				tc.firstPublished + `", "updated_at": "` + tc.updatedAt + `"}]}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(srv.Close)

			postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
			if err != nil {
				t.Fatalf("FetchPostings: %v", err)
			}
			if len(postings) != 1 {
				t.Fatalf("got %d postings, want 1", len(postings))
			}
			p := postings[0]
			if p.SourceFirstPublishedAt == nil || !p.SourceFirstPublishedAt.Equal(tc.wantFirst) {
				t.Errorf("SourceFirstPublishedAt: got %v, want %v", p.SourceFirstPublishedAt, tc.wantFirst)
			}
			if p.SourceLastModifiedAt == nil || !p.SourceLastModifiedAt.Equal(tc.wantUpdated) {
				t.Errorf("SourceLastModifiedAt: got %v, want %v", p.SourceLastModifiedAt, tc.wantUpdated)
			}
		})
	}
}

func TestGreenhouseAdapter_SourceTimestamps_BothAbsent(t *testing.T) {
	// Cover both absence shapes the adapter is meant to treat uniformly:
	// JSON null on first_published, missing key on updated_at, and
	// empty string on a third row. All three should land as nil fields,
	// no error.
	body := `{"jobs": [
		{"id": 1, "title": "Null Role", "first_published": null, "updated_at": null},
		{"id": 2, "title": "Missing Role"},
		{"id": 3, "title": "Empty Role", "first_published": "", "updated_at": ""}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 3 {
		t.Fatalf("got %d postings, want 3", len(postings))
	}
	for i, p := range postings {
		if p.SourceFirstPublishedAt != nil {
			t.Errorf("posting %d: SourceFirstPublishedAt: got pointer to %v, want nil", i, *p.SourceFirstPublishedAt)
		}
		if p.SourceLastModifiedAt != nil {
			t.Errorf("posting %d: SourceLastModifiedAt: got pointer to %v, want nil", i, *p.SourceLastModifiedAt)
		}
	}
}

func TestGreenhouseAdapter_SourceTimestamps_MalformedFirstPublished(t *testing.T) {
	body := `{"jobs": [{"id": 7, "title": "Role", "first_published": "not-a-date"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for malformed first_published")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
	msg := err.Error()
	if !strings.Contains(msg, "first_published") {
		t.Errorf("error %q does not name the wire field first_published", msg)
	}
	if strings.Contains(msg, "parse updated_at") {
		t.Errorf("error %q mentions parse updated_at — wire field name substitution should be first_published only", msg)
	}
	if !strings.Contains(msg, `"not-a-date"`) {
		t.Errorf("error %q does not contain the raw value %q (%%q-quoted)", msg, "not-a-date")
	}
}

func TestGreenhouseAdapter_SourceTimestamps_MalformedUpdatedAt(t *testing.T) {
	// updated_at malformed but first_published valid — confirms the %s
	// substitution carries the offending wire field name (not a hard-coded one).
	body := `{"jobs": [{
		"id": 8,
		"title": "Role",
		"first_published": "2026-04-17T12:21:54-04:00",
		"updated_at": "garbage"
	}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for malformed updated_at")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
	msg := err.Error()
	if !strings.Contains(msg, "updated_at") {
		t.Errorf("error %q does not name the wire field updated_at", msg)
	}
	// first_published parses fine here; the error must not point at it.
	if strings.Contains(msg, "parse first_published") {
		t.Errorf("error %q points at first_published — wire field name should be updated_at", msg)
	}
	if !strings.Contains(msg, `"garbage"`) {
		t.Errorf("error %q does not contain the raw value %q (%%q-quoted)", msg, "garbage")
	}
}

// TestGreenhouseAdapter_PopulatesDescriptionText pins the wiring from the
// Greenhouse `content` field through htmlToPlainText into Posting.DescriptionText.
// Inline JSON keeps the wire shape minimal and isolates the description path
// from unrelated fixture noise.
func TestGreenhouseAdapter_PopulatesDescriptionText(t *testing.T) {
	body := `{"jobs":[{"id":42,"title":"Role","content":"<p>Hello <b>world</b></p>"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	p := postings[0]
	if p.DescriptionText == nil {
		t.Fatalf("DescriptionText: got nil, want pointer to %q", "Hello world")
	}
	if got, want := *p.DescriptionText, "Hello world"; got != want {
		t.Errorf("DescriptionText: got %q, want %q", got, want)
	}
}

// TestGreenhouseAdapter_NilDescriptionTextWhenNoContent verifies that an empty
// `content` field leaves DescriptionText nil — the absence path matches the
// adapter contract (nil, not an empty pointer).
func TestGreenhouseAdapter_NilDescriptionTextWhenNoContent(t *testing.T) {
	body := `{"jobs":[{"id":42,"title":"Role","content":""}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	if postings[0].DescriptionText != nil {
		t.Errorf("DescriptionText: got pointer to %q, want nil", *postings[0].DescriptionText)
	}
}

// TestGreenhouseAdapter_NilCompensation pins the Task 4 spike finding: no live
// Greenhouse board exposes pay_input_ranges, so all four compensation fields
// always come back nil. If a future board surfaces structured comp, this test
// will start failing and force the parsing path to be implemented (not silently
// drop data).
func TestGreenhouseAdapter_NilCompensation(t *testing.T) {
	body := `{"jobs":[{"id":42,"title":"Role","content":"<p>desc</p>","pay_input_ranges":[{"min":150000,"max":200000,"currency":"USD"}]}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
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

func TestGreenhouseAdapter_OversizeResponse_ReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 32 MiB allocation in -short mode")
	}
	// Response body exceeds maxResponseBytes; adapter must return an error rather
	// than attempting to parse a potentially OOM-inducing payload.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Write maxResponseBytes+1 bytes. The adapter reads up to maxResponseBytes+1
		// to detect overflow; seeing that many bytes means the body was truncated.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, maxResponseBytes+1))
	}))
	t.Cleanup(srv.Close)

	postings, err := newGreenhouseWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "exampleco")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want error for oversize response")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil on error", postings)
	}
}
