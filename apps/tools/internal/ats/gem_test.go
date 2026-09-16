package ats

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	gemSupioToken = "supio"
	gemWantPath   = "/supio/job_posts/"
)

func TestGemAdapter_SupioFixture(t *testing.T) {
	fixture := loadAdapterFixture(t, "gem", "jobs_full.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %q, want GET", r.Method)
		}
		if r.URL.Path != gemWantPath {
			t.Errorf("path: got %q, want %q", r.URL.Path, gemWantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 7 {
		t.Fatalf("got %d postings, want 7", len(postings))
	}

	// This live hybrid posting has a label-only Seattle office and an explicit
	// San Francisco office location. Only the latter belongs in LocationTexts.
	p := postings[0]
	wantURL := "https://jobs.gem.com/supio/am9icG9zdDqse7B07y815DKTEfT06Ipv"
	if p.SourceURL != wantURL {
		t.Errorf("SourceURL: got %q, want %q", p.SourceURL, wantURL)
	}
	if p.JobURL == nil || *p.JobURL != wantURL {
		t.Errorf("JobURL: got %v, want pointer to %q", p.JobURL, wantURL)
	}
	if p.LocationText == nil || *p.LocationText != "San Francisco, United States" {
		t.Errorf("LocationText: got %v, want San Francisco, United States", p.LocationText)
	}
	if len(p.LocationTexts) != 1 || p.LocationTexts[0] != "San Francisco, United States" {
		t.Errorf("LocationTexts: got %v, want [San Francisco, United States]", p.LocationTexts)
	}
	if p.WorkplaceType == nil || *p.WorkplaceType != "hybrid" {
		t.Errorf("WorkplaceType: got %v, want hybrid", p.WorkplaceType)
	}
	if p.EmploymentType == nil || *p.EmploymentType != "full_time" {
		t.Errorf("EmploymentType: got %v, want full_time", p.EmploymentType)
	}
	if p.DescriptionText == nil || *p.DescriptionText == "" {
		t.Errorf("DescriptionText: got %v, want content_plain text", p.DescriptionText)
	}

	wantPublishedAt := time.Date(2026, 1, 28, 22, 56, 46, 609000000, time.UTC)
	if p.PostedAt == nil || !p.PostedAt.Equal(wantPublishedAt) {
		t.Errorf("PostedAt: got %v, want %v", p.PostedAt, wantPublishedAt)
	}
	if p.SourceFirstPublishedAt == nil || !p.SourceFirstPublishedAt.Equal(wantPublishedAt) {
		t.Errorf("SourceFirstPublishedAt: got %v, want %v", p.SourceFirstPublishedAt, wantPublishedAt)
	}
	// created_at is three minutes earlier on this recorded job. The matching
	// PostedAt and SourceFirstPublishedAt values prove first_published_at, not
	// record creation, supplies both fields.
	createdAt := time.Date(2026, 1, 28, 22, 53, 48, 104000000, time.UTC)
	if p.PostedAt != nil && p.PostedAt.Equal(createdAt) {
		t.Errorf("PostedAt: got created_at %v, want first_published_at %v", createdAt, wantPublishedAt)
	}
	wantUpdatedAt := time.Date(2026, 3, 13, 5, 58, 54, 877000000, time.UTC)
	if p.SourceLastModifiedAt == nil || !p.SourceLastModifiedAt.Equal(wantUpdatedAt) {
		t.Errorf("SourceLastModifiedAt: got %v, want %v", p.SourceLastModifiedAt, wantUpdatedAt)
	}
	for i, posting := range postings {
		if posting.CompensationMin != nil || posting.CompensationMax != nil || posting.CompensationCurrency != nil || posting.CompensationPeriod != nil {
			t.Errorf("posting %d compensation: got %+v, want all nil", i, posting)
		}
	}

	var rawJobs []json.RawMessage
	if err := json.Unmarshal(fixture, &rawJobs); err != nil {
		t.Fatalf("decode fixture raw jobs: %v", err)
	}
	if !bytes.Equal(p.RawData, rawJobs[0]) {
		t.Errorf("RawData does not preserve the original per-job bytes")
	}
}

func TestGemAdapter_GemBoardPreservesNumericIDAndMapsInOffice(t *testing.T) {
	fixture := loadAdapterFixture(t, "gem", "jobs_gem_board.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), "gem")
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 4 {
		t.Fatalf("got %d postings, want 4", len(postings))
	}

	var numericIDFound bool
	var inOfficePostingFound bool
	for _, p := range postings {
		if p.SourceID == "4965519002" {
			numericIDFound = true
		}
		if p.SourceID == "am9icG9zdDqbFFvbAtTFWyEIqIoh7MQe" {
			inOfficePostingFound = true
			if p.WorkplaceType == nil || *p.WorkplaceType != "onsite" {
				t.Errorf("in_office posting WorkplaceType: got %v, want onsite", p.WorkplaceType)
			}
		}
	}
	if !numericIDFound {
		t.Error("numeric Gem id 4965519002 was not preserved as SourceID")
	}
	if !inOfficePostingFound {
		t.Error("in_office Gem posting was not returned")
	}
}

func TestGemAdapter_EmptyBoard(t *testing.T) {
	fixture := loadAdapterFixture(t, "gem", "jobs_empty.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if len(postings) != 0 {
		t.Errorf("got %d postings, want 0", len(postings))
	}
}

func TestGemAdapter_MissingRequiredField(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		field   string
	}{
		{name: "id", fixture: "jobs_missing_id.json", field: "id"},
		{name: "absolute url", fixture: "jobs_missing_absolute_url.json", field: "absolute_url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := loadAdapterFixture(t, "gem", tc.fixture)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(fixture)
			}))
			t.Cleanup(srv.Close)

			_, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
			if err == nil {
				t.Fatal("FetchPostings: got nil error, want non-nil")
			}
			for _, want := range []string{"gem:", "index 0", tc.field} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestGemAdapter_MalformedTimestampAbortsFetch(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		field   string
	}{
		{name: "first published at", fixture: "jobs_invalid_first_published_at.json", field: "first_published_at"},
		{name: "updated at", fixture: "jobs_invalid_updated_at.json", field: "updated_at"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := loadAdapterFixture(t, "gem", tc.fixture)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(fixture)
			}))
			t.Cleanup(srv.Close)

			postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
			if err == nil {
				t.Fatal("FetchPostings: got nil error, want non-nil")
			}
			if postings != nil {
				t.Errorf("postings: got %v, want nil after malformed timestamp", postings)
			}
			for _, want := range []string{"gem:", "index 0", tc.field} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestGemAdapter_NoLocationsLeavesFieldsNil(t *testing.T) {
	fixture := loadAdapterFixture(t, "gem", "jobs_no_locations.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	if postings[0].LocationText != nil || postings[0].LocationTexts != nil {
		t.Errorf("locations: got LocationText=%v LocationTexts=%v, want both nil", postings[0].LocationText, postings[0].LocationTexts)
	}
}

func TestGemAdapter_ContentPlainFallback(t *testing.T) {
	fixture := loadAdapterFixture(t, "gem", "jobs_no_content_plain.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}
	var jobs []struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(fixture, &jobs); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	want := htmlToPlainText(jobs[0].Content)
	if postings[0].DescriptionText == nil || *postings[0].DescriptionText != want {
		t.Errorf("DescriptionText: got %v, want html fallback %q", postings[0].DescriptionText, want)
	}
}

func TestGemAdapter_InvalidBoardTokenSkipsHTTP(t *testing.T) {
	tests := []string{"", "   ", "supio/other", "https://supio", "supio?foo=bar", "supio#section"}
	for _, token := range tests {
		t.Run(token, func(t *testing.T) {
			var requestCount int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&requestCount, 1)
				_, _ = w.Write([]byte("[]"))
			}))
			t.Cleanup(srv.Close)

			postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), token)
			if err == nil || !strings.Contains(err.Error(), "gem:") {
				t.Errorf("FetchPostings error = %v, want gem-prefixed error", err)
			}
			if postings != nil {
				t.Errorf("postings: got %v, want nil", postings)
			}
			if got := atomic.LoadInt32(&requestCount); got != 0 {
				t.Errorf("request count: got %d, want 0", got)
			}
		})
	}
}

// TestGemAdapter_RequisitionKeyFromInternalJobID pins the requisition key to
// Gem's `internal_job_id`. Gem draws the job-versus-job-post distinction: on
// this recorded board every `id` base64-decodes to "jobpost:…" and every
// `internal_job_id` to "job:…", so the two are different identifiers and the
// key must never be a copy of SourceID.
func TestGemAdapter_RequisitionKeyFromInternalJobID(t *testing.T) {
	fixture := loadAdapterFixture(t, "gem", "jobs_full.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
	if err != nil {
		t.Fatalf("FetchPostings: %v", err)
	}

	want := []struct {
		sourceID       string
		requisitionKey string
	}{
		{"am9icG9zdDqse7B07y815DKTEfT06Ipv", "am9iOmUBlETzb9qpRozEbBOuonc"},
		{"am9icG9zdDoD_2EMxypZVF1uQCvSoMu5", "am9iOq5k6zDaEq3Gg4dMVgR-jyo"},
		{"am9icG9zdDp_0pnMKN0lg55LNclgvKRe", "am9iOpch5t43aO1ixCdgDSmcq68"},
		{"am9icG9zdDq7TcZ2MK-GVbcwAbMMPcsM", "am9iOpMR6ySBupEfKRPwDVZLa1k"},
		{"am9icG9zdDpNipDbaZHs89GjmNb6F4IO", "am9iOqCwER_efx-JZBwMhi-5YtQ"},
		{"am9icG9zdDqA6mT47ZS3M7f6TeG6qv36", "am9iOvrMPb6xviOENmCcAbJm5BM"},
		{"am9icG9zdDpGbtVkYuwqKdDSBHx74oNl", "am9iOiQc5Cq5yyBGS0WB7QWlxFk"},
	}
	if len(postings) != len(want) {
		t.Fatalf("got %d postings, want %d", len(postings), len(want))
	}
	for i, tc := range want {
		p := postings[i]
		if p.SourceID != tc.sourceID {
			t.Errorf("posting %d: SourceID: got %q, want %q", i, p.SourceID, tc.sourceID)
		}
		switch {
		case p.RequisitionKey == nil:
			t.Errorf("posting %d (%s): RequisitionKey: got nil, want pointer to %q", i, tc.sourceID, tc.requisitionKey)
		case *p.RequisitionKey != tc.requisitionKey:
			t.Errorf("posting %d (%s): RequisitionKey: got %q, want %q", i, tc.sourceID, *p.RequisitionKey, tc.requisitionKey)
		}
		if p.RequisitionKey != nil && *p.RequisitionKey == p.SourceID {
			t.Errorf("posting %d: RequisitionKey equals SourceID (%q) — the key must be the job, not the job post", i, p.SourceID)
		}
	}
}

// TestGemAdapter_BlankInternalJobID_LeavesRequisitionKeyNil covers the three
// absence shapes no recorded board carries. All must land as nil: a
// whitespace-only key would otherwise be shared by every such post and stamp
// them as one ATS-supplied requisition.
func TestGemAdapter_BlankInternalJobID_LeavesRequisitionKeyNil(t *testing.T) {
	const base = `{"id":"am9icG9zdDpA","absolute_url":"https://jobs.gem.com/supio/am9icG9zdDpA","title":"Role"`
	cases := []struct {
		name string
		job  string
	}{
		{"absent", base + `}`},
		{"null", base + `,"internal_job_id":null}`},
		{"empty_string", base + `,"internal_job_id":""}`},
		{"whitespace_only", base + `,"internal_job_id":"   "}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`[` + tc.job + `]`))
			}))
			t.Cleanup(srv.Close)

			postings, err := newGemWithBaseURL(srv.Client(), srv.URL).FetchPostings(t.Context(), gemSupioToken)
			if err != nil {
				t.Fatalf("FetchPostings: %v", err)
			}
			if len(postings) != 1 {
				t.Fatalf("got %d postings, want 1", len(postings))
			}
			if postings[0].RequisitionKey != nil {
				t.Errorf("RequisitionKey: got pointer to %q, want nil", *postings[0].RequisitionKey)
			}
		})
	}
}
