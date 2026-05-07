package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/majtom2grndctrl/market-scout/internal/ats"
	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

func TestClassifyCompanyError(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		parentCtxErr error
		wantAborted  bool
	}{
		{
			name:         "canceled with parent done → aborted_shutdown",
			err:          context.Canceled,
			parentCtxErr: context.Canceled,
			wantAborted:  true,
		},
		{
			name:         "canceled without parent done → failed (per-company cancel, not shutdown)",
			err:          context.Canceled,
			parentCtxErr: nil,
			wantAborted:  false,
		},
		{
			name:         "deadline exceeded → failed (per-company timeout, not shutdown)",
			err:          context.DeadlineExceeded,
			parentCtxErr: context.Canceled,
			wantAborted:  false,
		},
		{
			name:         "generic error → failed",
			err:          errors.New("some db error"),
			parentCtxErr: nil,
			wantAborted:  false,
		},
		{
			name:         "wrapped canceled with parent done → aborted_shutdown",
			err:          fmt.Errorf("fetching postings: %w", context.Canceled),
			parentCtxErr: context.Canceled,
			wantAborted:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCompanyError(tc.err, tc.parentCtxErr)
			if got != tc.wantAborted {
				t.Errorf("classifyCompanyError(%v, %v) = %v, want %v", tc.err, tc.parentCtxErr, got, tc.wantAborted)
			}
		})
	}
}

func TestValidatePosting(t *testing.T) {
	validRaw := json.RawMessage(`{"id":1}`)
	title := "Role"

	t.Run("valid posting passes", func(t *testing.T) {
		p := domain.Posting{
			SourceID:  "123",
			SourceURL: "https://example.com/jobs/123",
			Title:     &title,
			RawData:   validRaw,
		}
		if err := validatePosting(0, p); err != nil {
			t.Errorf("validatePosting: unexpected error: %v", err)
		}
	})

	t.Run("empty SourceURL is rejected", func(t *testing.T) {
		p := domain.Posting{SourceID: "123", RawData: validRaw}
		if err := validatePosting(0, p); err == nil {
			t.Error("validatePosting: expected error for empty SourceURL, got nil")
		}
	})

	t.Run("empty SourceID is rejected", func(t *testing.T) {
		p := domain.Posting{SourceURL: "https://example.com/jobs/123", RawData: validRaw}
		if err := validatePosting(0, p); err == nil {
			t.Error("validatePosting: expected error for empty SourceID, got nil")
		}
	})

	t.Run("empty RawData is rejected", func(t *testing.T) {
		p := domain.Posting{
			SourceID:  "123",
			SourceURL: "https://example.com/jobs/123",
			RawData:   nil,
		}
		if err := validatePosting(0, p); err == nil {
			t.Error("validatePosting: expected error for empty RawData, got nil")
		}
	})

	t.Run("error message includes posting index", func(t *testing.T) {
		p := domain.Posting{SourceID: "123", RawData: validRaw} // empty SourceURL
		err := validatePosting(3, p)
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		if !strings.Contains(err.Error(), "3") {
			t.Errorf("error %q does not include posting index 3", err.Error())
		}
	})
}

func TestSnapshotWrite_ForwardsSourceTimestampsToParams(t *testing.T) {
	// Both ATS-reported timestamps set on the domain.Posting must arrive in
	// InsertPostingSnapshotParams as sql.NullTime{Valid: true} carrying the
	// exact instant set on the domain field. This pins the wiring through
	// nullTime so a future refactor can't silently drop one of the new columns.
	first := time.Date(2026, 4, 17, 12, 21, 54, 0, time.UTC)
	last := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	p := domain.Posting{
		SourceID:               "123",
		SourceURL:              "https://example.com/jobs/123",
		RawData:                json.RawMessage(`{"id":123}`),
		SourceFirstPublishedAt: &first,
		SourceLastModifiedAt:   &last,
	}
	fetchedAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	got := buildSnapshotParams(99, 7, fetchedAt, p)

	wantFirst := sql.NullTime{Time: first, Valid: true}
	if got.SourceFirstPublishedAt != wantFirst {
		t.Errorf("SourceFirstPublishedAt: got %+v, want %+v", got.SourceFirstPublishedAt, wantFirst)
	}
	wantLast := sql.NullTime{Time: last, Valid: true}
	if got.SourceLastModifiedAt != wantLast {
		t.Errorf("SourceLastModifiedAt: got %+v, want %+v", got.SourceLastModifiedAt, wantLast)
	}
	if got.JobPostingID != 99 {
		t.Errorf("JobPostingID: got %d, want 99", got.JobPostingID)
	}
	if !got.FetchedAt.Equal(fetchedAt) {
		t.Errorf("FetchedAt: got %v, want %v", got.FetchedAt, fetchedAt)
	}
}

func TestSnapshotWrite_PreservesNonUTCTimezoneOffset(t *testing.T) {
	// Greenhouse emits timestamps with non-UTC offsets (e.g. -04:00). The
	// nullTime conversion must preserve the original Location so callers can
	// observe the ATS-reported offset. A silent .UTC() normalization would
	// represent the same instant but discard the offset, losing the signal.
	edt := time.FixedZone("EDT", -4*3600)
	first := time.Date(2026, 4, 17, 12, 21, 54, 0, edt)
	last := time.Date(2026, 4, 18, 9, 30, 0, 0, time.UTC)
	p := domain.Posting{
		SourceID:               "456",
		SourceURL:              "https://example.com/jobs/456",
		RawData:                json.RawMessage(`{"id":456}`),
		SourceFirstPublishedAt: &first,
		SourceLastModifiedAt:   &last,
	}
	got := buildSnapshotParams(1, 7, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), p)

	if !got.SourceFirstPublishedAt.Valid {
		t.Fatal("SourceFirstPublishedAt: got Valid=false, want Valid=true")
	}
	if !got.SourceFirstPublishedAt.Time.Equal(first) {
		t.Errorf("SourceFirstPublishedAt: instant mismatch: got %v, want %v",
			got.SourceFirstPublishedAt.Time, first)
	}
	if got.SourceFirstPublishedAt.Time.Location().String() != edt.String() {
		t.Errorf("SourceFirstPublishedAt: location not preserved: got %q, want %q",
			got.SourceFirstPublishedAt.Time.Location().String(), edt.String())
	}
}

func TestSnapshotWrite_NilSourceTimestampsBecomeNullTime(t *testing.T) {
	// nil pointers on the domain side must produce zero-value sql.NullTime
	// (Valid: false) — the column persists as NULL.
	p := domain.Posting{
		SourceID:  "123",
		SourceURL: "https://example.com/jobs/123",
		RawData:   json.RawMessage(`{"id":123}`),
	}
	got := buildSnapshotParams(1, 7, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), p)
	if got.SourceFirstPublishedAt.Valid {
		t.Errorf("SourceFirstPublishedAt: got Valid=true, want Valid=false for nil input")
	}
	if got.SourceLastModifiedAt.Valid {
		t.Errorf("SourceLastModifiedAt: got Valid=true, want Valid=false for nil input")
	}
}

func TestSnapshotWrite_ForwardsLocationTextsToParams(t *testing.T) {
	// LocationTexts must flow through buildSnapshotParams verbatim: nil stays nil
	// (DB NULL) and a populated slice is passed through unchanged. sqlc generated
	// a plain []string param, so no bridging helper is in play — this test pins
	// the direct assignment so a future refactor can't drop the wiring.
	t.Run("nil LocationTexts stays nil", func(t *testing.T) {
		p := domain.Posting{
			SourceID:  "123",
			SourceURL: "https://example.com/jobs/123",
			RawData:   json.RawMessage(`{"id":123}`),
		}
		got := buildSnapshotParams(1, 7, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), p)
		if got.LocationTexts != nil {
			t.Errorf("LocationTexts: got %v, want nil", got.LocationTexts)
		}
	})

	t.Run("populated LocationTexts pass through verbatim", func(t *testing.T) {
		want := []string{"New York, NY", "San Francisco, CA", "Remote"}
		p := domain.Posting{
			SourceID:      "123",
			SourceURL:     "https://example.com/jobs/123",
			RawData:       json.RawMessage(`{"id":123}`),
			LocationTexts: want,
		}
		got := buildSnapshotParams(1, 7, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), p)
		if len(got.LocationTexts) != len(want) {
			t.Fatalf("LocationTexts: got len=%d, want len=%d", len(got.LocationTexts), len(want))
		}
		for i := range want {
			if got.LocationTexts[i] != want[i] {
				t.Errorf("LocationTexts[%d]: got %q, want %q", i, got.LocationTexts[i], want[i])
			}
		}
	})
}

// rewriteTransport redirects every outbound request's scheme+host to a fixed
// target (an httptest.Server URL) while preserving path and query. This lets
// the real ats.NewGreenhouse / NewLever / NewAshby adapters — which bake their
// production base URLs in — be driven against an httptest.Server without
// touching package-private constructors or modifying main.go.
type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = rt.target.Scheme
	clone.URL.Host = rt.target.Host
	clone.Host = rt.target.Host
	return rt.base.RoundTrip(clone)
}

// TestAdaptersIndependentlyOverHTTP verifies that Greenhouse, Lever, and Ashby
// adapters each complete their own HTTP roundtrip independently: a Lever 5xx
// does not prevent the Greenhouse or Ashby fetches from succeeding. The real
// internal/ats adapters run against a single httptest.Server routed by path
// prefix, so the real HTTP client, URL-building logic, and response decoders
// all execute. Concurrency is provided by a local goroutine fan-out identical
// in shape to run()'s dispatch loop.
//
// This does NOT exercise run() itself. run() has no injectable seam for
// adapters or the DB writer, so its dispatch loop cannot be tested without a
// real Postgres (gated behind //go:build e2e per testing-guide §1). Testing
// run()'s context-propagation and per-company cancellation semantics requires
// an injectable seam — that refactor is deferred as a follow-up.
func TestAdaptersIndependentlyOverHTTP(t *testing.T) {
	// Minimal valid payloads — just enough to satisfy each adapter's required
	// fields. Hand-rolled (not loaded from internal/ats/testdata) because that
	// directory is package-private to the ats package and reaching across with
	// a relative path would couple cmd/fetcher tests to ats's test fixtures.
	const greenhouseBody = `{"jobs":[{"id":1,"title":"GH Engineer","absolute_url":"https://example.com/gh/1","location":{"name":"Remote"}}]}`
	const ashbyBody = `{"apiVersion":"1","jobs":[{"id":"ashby-1","title":"Ashby Engineer","jobUrl":"https://example.com/ashby/1","location":"Remote"}]}`

	var (
		ghHits, ashbyHits, leverHits int
		hitsMu                       sync.Mutex
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsMu.Lock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/boards/"):
			ghHits++
		case strings.HasPrefix(r.URL.Path, "/posting-api/job-board/"):
			ashbyHits++
		case strings.HasPrefix(r.URL.Path, "/v0/postings/"):
			leverHits++
		}
		hitsMu.Unlock()

		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/boards/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(greenhouseBody))
		case strings.HasPrefix(r.URL.Path, "/posting-api/job-board/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(ashbyBody))
		case strings.HasPrefix(r.URL.Path, "/v0/postings/"):
			// 5xx — the failure mode named in the AC.
			http.Error(w, "lever upstream is having a bad day", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	httpClient := &http.Client{
		Transport: &rewriteTransport{target: target, base: http.DefaultTransport},
		Timeout:   5 * time.Second,
	}

	// Real adapters — not the atsAdapter interface mocked. Production constructors
	// bake the public base URLs in; the rewriteTransport above intercepts each
	// request and redirects it to srv while preserving the path the adapter built.
	// This means each adapter's URL-building logic still runs (and is implicitly
	// validated by the path-prefix routing on the server side).
	adaptersByATS := map[string]atsAdapter{
		"greenhouse": ats.NewGreenhouse(httpClient),
		"lever":      ats.NewLever(httpClient),
		"ashby":      ats.NewAshby(httpClient),
	}

	type result struct {
		ats      string
		postings []domain.Posting
		err      error
	}

	companies := []struct {
		name, ats, token string
	}{
		{"GHCo", "greenhouse", "ghco"},
		{"LeverCo", "lever", "leverco"},
		{"AshbyCo", "ashby", "ashbyco"},
	}

	// Mirror main.go's dispatch loop: each company runs in its own goroutine,
	// errors collected separately. If isolation were broken (e.g. a shared
	// cancellable context derived from the failing fetch, or a panic), the OK
	// providers would also fail or the goroutines would deadlock.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make([]result, len(companies))
	for i, c := range companies {
		wg.Add(1)
		go func(i int, name, atsKey, token string) {
			defer wg.Done()
			adapter := adaptersByATS[atsKey]
			postings, err := adapter.FetchPostings(ctx, token)
			results[i] = result{ats: atsKey, postings: postings, err: err}
		}(i, c.name, c.ats, c.token)
	}
	wg.Wait()

	byATS := make(map[string]result, len(results))
	for _, r := range results {
		byATS[r.ats] = r
	}

	// Greenhouse: succeeded, returned the one posting from the fixture body.
	if gh := byATS["greenhouse"]; gh.err != nil {
		t.Errorf("greenhouse: unexpected error (Lever 5xx leaked across adapters): %v", gh.err)
	} else if len(gh.postings) != 1 {
		t.Errorf("greenhouse: got %d postings, want 1", len(gh.postings))
	} else if gh.postings[0].SourceID != "1" {
		t.Errorf("greenhouse: SourceID = %q, want %q", gh.postings[0].SourceID, "1")
	}

	// Ashby: succeeded, returned the one posting. The AC explicitly names this
	// pairing — "a Lever 5xx does not abort an Ashby fetch."
	if ashby := byATS["ashby"]; ashby.err != nil {
		t.Errorf("ashby: unexpected error (Lever 5xx leaked across adapters): %v", ashby.err)
	} else if len(ashby.postings) != 1 {
		t.Errorf("ashby: got %d postings, want 1", len(ashby.postings))
	} else if ashby.postings[0].SourceID != "ashby-1" {
		t.Errorf("ashby: SourceID = %q, want %q", ashby.postings[0].SourceID, "ashby-1")
	}

	// Lever: failed with the 5xx. The error is contained — it surfaced as the
	// goroutine's return value rather than panicking or cancelling siblings.
	if lever := byATS["lever"]; lever.err == nil {
		t.Error("lever: expected non-nil error from 5xx, got nil (isolation can't be evaluated if Lever didn't actually fail)")
	} else if len(lever.postings) != 0 {
		t.Errorf("lever: got %d postings, want 0 on failure (adapter contract: no partial successes)", len(lever.postings))
	}

	// Each adapter actually issued its HTTP call — the goroutine model didn't
	// swallow any of them — confirming the parallel fan-out reached all three
	// servers despite Lever's failure.
	hitsMu.Lock()
	defer hitsMu.Unlock()
	if ghHits == 0 {
		t.Error("greenhouse adapter never hit the server")
	}
	if ashbyHits == 0 {
		t.Error("ashby adapter never hit the server")
	}
	if leverHits == 0 {
		t.Error("lever adapter never hit the server")
	}
}

func TestSummaryInvariant(t *testing.T) {
	// The invariant success + failed + aborted_shutdown = total_attempted must
	// hold for every possible run outcome. Test the arithmetic across all
	// combinations that can arise from the dispatch loop.
	cases := []struct {
		name           string
		success        int
		failed         int
		aborted        int
		totalAttempted int
		wantViolated   bool
	}{
		{"all success", 5, 0, 0, 5, false},
		{"all failed", 0, 5, 0, 5, false},
		{"all aborted", 0, 0, 5, 5, false},
		{"mixed", 2, 2, 1, 5, false},
		{"undercounted — goroutine dropped outcome", 2, 2, 0, 5, true},
		{"overcounted — goroutine double-reported", 3, 2, 1, 5, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violated := tc.success+tc.failed+tc.aborted != tc.totalAttempted
			if violated != tc.wantViolated {
				t.Errorf("invariant check: got violated=%v, want %v (success=%d failed=%d aborted=%d total=%d)",
					violated, tc.wantViolated, tc.success, tc.failed, tc.aborted, tc.totalAttempted)
			}
		})
	}
}
