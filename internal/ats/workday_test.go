package ats

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	workdayBoardToken = "nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite"
	workdayWantPath   = "/wday/cxs/nvidia/NVIDIAExternalCareerSite/jobs"
)

// decodeWorkdayRequest pulls the offset (and validates method/path) from an
// inbound POST. Returns the parsed offset.
func decodeWorkdayRequest(t *testing.T, r *http.Request) int {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("method: got %q, want POST", r.Method)
	}
	if r.URL.Path != workdayWantPath {
		t.Errorf("path: got %q, want %q", r.URL.Path, workdayWantPath)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var req struct {
		Limit         int            `json:"limit"`
		Offset        int            `json:"offset"`
		SearchText    string         `json:"searchText"`
		AppliedFacets map[string]any `json:"appliedFacets"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body %q: %v", body, err)
	}
	return req.Offset
}

func TestWorkdayAdapter_SuccessfulParse(t *testing.T) {
	fixture := loadAdapterFixture(t, "workday", "jobs_page1.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeWorkdayRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkdayWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workdayBoardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}

	p := postings[0]
	wantSourceID := "/job/US-CA-Santa-Clara/Senior-Software-Engineer_JR12345"
	if p.SourceID != wantSourceID {
		t.Errorf("SourceID: got %q, want %q", p.SourceID, wantSourceID)
	}
	wantURL := "https://nvidia.wd5.myworkdayjobs.com/en-US/NVIDIAExternalCareerSite/job/US-CA-Santa-Clara/Senior-Software-Engineer_JR12345"
	if p.SourceURL != wantURL {
		t.Errorf("SourceURL: got %q, want %q", p.SourceURL, wantURL)
	}
	if p.JobURL == nil || *p.JobURL != wantURL {
		t.Errorf("JobURL: got %v, want pointer to %q", p.JobURL, wantURL)
	}
	if p.Title == nil || *p.Title != "Senior Software Engineer" {
		t.Errorf("Title: got %v, want pointer to %q", p.Title, "Senior Software Engineer")
	}
	if p.LocationText == nil || *p.LocationText != "Santa Clara, CA" {
		t.Errorf("LocationText: got %v, want pointer to %q", p.LocationText, "Santa Clara, CA")
	}
	if len(p.LocationTexts) != 1 || p.LocationTexts[0] != "Santa Clara, CA" {
		t.Errorf("LocationTexts: got %v, want [%q]", p.LocationTexts, "Santa Clara, CA")
	}
	wantPostedAt := time.Date(2024, 4, 25, 0, 0, 0, 0, time.UTC)
	if p.PostedAt == nil || !p.PostedAt.Equal(wantPostedAt) {
		t.Errorf("PostedAt: got %v, want %v", p.PostedAt, wantPostedAt)
	}
	if p.SourceFirstPublishedAt == nil || !p.SourceFirstPublishedAt.Equal(wantPostedAt) {
		t.Errorf("SourceFirstPublishedAt: got %v, want %v", p.SourceFirstPublishedAt, wantPostedAt)
	}
	if p.DescriptionText != nil {
		t.Errorf("DescriptionText: got %v, want nil (v1 omits descriptions)", *p.DescriptionText)
	}
	if p.CompensationMin != nil {
		t.Errorf("CompensationMin: got %v, want nil", *p.CompensationMin)
	}
}

func TestWorkdayAdapter_PaginatesUntilShortPage(t *testing.T) {
	page1 := loadAdapterFixture(t, "workday", "jobs_page1_full.json")
	page2 := loadAdapterFixture(t, "workday", "jobs_page2_short.json")

	var requestCount int32
	var sawOffset20 int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		offset := decodeWorkdayRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		switch offset {
		case 0:
			_, _ = w.Write(page1)
		case 20:
			atomic.StoreInt32(&sawOffset20, 1)
			_, _ = w.Write(page2)
		default:
			t.Errorf("unexpected offset: %d", offset)
			http.Error(w, "unexpected offset", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkdayWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workdayBoardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 25 {
		t.Fatalf("got %d postings, want 25", len(postings))
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Errorf("request count: got %d, want 2", got)
	}
	if atomic.LoadInt32(&sawOffset20) != 1 {
		t.Errorf("expected page 2 request with offset=20")
	}
}

func TestWorkdayAdapter_StopsWhenOffsetExceedsTotal(t *testing.T) {
	// Verifies the offset+len >= total termination path. Both exit conditions
	// fire on page 2 (page is short AND offset+len equals total); confirms no
	// third request fires.
	page1 := loadAdapterFixture(t, "workday", "jobs_page1_full.json")
	page2 := loadAdapterFixture(t, "workday", "jobs_page2_short.json")

	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		offset := decodeWorkdayRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		switch offset {
		case 0:
			_, _ = w.Write(page1)
		case 20:
			_, _ = w.Write(page2)
		case 25:
			t.Errorf("adapter requested offset=25 — should have terminated when offset+len >= total")
			_, _ = w.Write([]byte(`{"total":25,"jobPostings":[]}`))
		default:
			t.Errorf("unexpected offset: %d", offset)
			http.Error(w, "unexpected offset", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkdayWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workdayBoardToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 25 {
		t.Fatalf("got %d postings, want 25", len(postings))
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Errorf("request count: got %d, want 2 (must not issue a third request)", got)
	}
}

func TestWorkdayAdapter_SourceURLFallback(t *testing.T) {
	fixture := loadAdapterFixture(t, "workday", "jobs_no_external_url.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// path uses tenant=testco, site=TestSite from the board token
		if got, want := r.URL.Path, "/wday/cxs/testco/TestSite/jobs"; got != want {
			t.Errorf("path: got %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkdayWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "testco.wd1.myworkdayjobs.com/TestSite")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("got %d postings, want 1", len(postings))
	}
	wantURL := "https://testco.wd1.myworkdayjobs.com/en-US/TestSite/req/12345/some-job"
	if postings[0].SourceURL != wantURL {
		t.Errorf("SourceURL: got %q, want %q (fallback)", postings[0].SourceURL, wantURL)
	}
	if postings[0].JobURL == nil || *postings[0].JobURL != wantURL {
		t.Errorf("JobURL: got %v, want pointer to %q", postings[0].JobURL, wantURL)
	}
}

func TestWorkdayAdapter_InvalidBoardToken(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		_, _ = w.Write([]byte(`{"total":0,"jobPostings":[]}`))
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkdayWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "invalidtoken")
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for invalid board token")
	}
	if postings != nil {
		t.Errorf("postings: got %v, want nil", postings)
	}
	if got := atomic.LoadInt32(&requestCount); got != 0 {
		t.Errorf("request count: got %d, want 0 (no HTTP request should be made)", got)
	}
}

func TestWorkdayAdapter_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	postings, err := newWorkdayWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workdayBoardToken)
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for HTTP 500")
	}
	if len(postings) != 0 {
		t.Errorf("postings: got %d, want 0", len(postings))
	}
}

func TestWorkdayAdapter_MissingExternalPath(t *testing.T) {
	fixture := loadAdapterFixture(t, "workday", "jobs_missing_external_path.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	_, err := newWorkdayWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), workdayBoardToken)
	if err == nil {
		t.Fatalf("FetchPostings: got nil error, want non-nil for missing externalPath")
	}
	if !strings.Contains(err.Error(), "index 0") {
		t.Errorf("error %q does not identify offending job (expected substring %q)", err.Error(), "index 0")
	}
}
