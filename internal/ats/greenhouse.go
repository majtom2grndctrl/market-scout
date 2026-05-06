// Package ats hosts Greenhouse and future ATS adapter implementations.
// Adapters return domain.Posting from internal/domain.
// The consumer-side interface (atsAdapter) lives in cmd/fetcher — concrete
// adapters satisfy it implicitly via Go structural typing.
// See: agent-context/lib/project.md
package ats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

const greenhousePublicBaseURL = "https://boards-api.greenhouse.io/v1/boards"

// Response body size caps. The 32 MiB ceiling guards against OOM from a
// pathological response; the 4 KiB cap on error snippets keeps wrapped error
// strings bounded while preserving enough body to debug 4xx/5xx replies.
const (
	maxResponseBytes = 32 * 1024 * 1024
	maxErrBodyBytes  = 4 * 1024
)

// Greenhouse reads from the Greenhouse public Job Board API.
// The API requires no auth and returns all jobs for a board in a single request.
type Greenhouse struct {
	client  *http.Client
	baseURL string
}

// New returns a Greenhouse adapter. If client is nil, http.DefaultClient is used.
// The adapter does not set a client-level timeout; cancellation flows from the context
// passed to FetchPostings.
func New(client *http.Client) *Greenhouse {
	return newWithBaseURL(client, greenhousePublicBaseURL)
}

// newWithBaseURL constructs a Greenhouse adapter pointed at an arbitrary base URL.
// Test-only: lets adapter tests target an httptest.Server without mutating
// package-level state. Production callers use New.
func newWithBaseURL(client *http.Client, baseURL string) *Greenhouse {
	if client == nil {
		client = http.DefaultClient
	}
	return &Greenhouse{client: client, baseURL: baseURL}
}

// ghResponse mirrors the wire shape of the Greenhouse jobs endpoint.
// Jobs are kept as raw messages so the original bytes can be preserved as RawData
// without losing fields the typed wire struct does not declare.
type ghResponse struct {
	Jobs []json.RawMessage `json:"jobs"`
}

// ghJob is the subset of the per-job wire shape the adapter extracts into Posting.
// PostedAt is intentionally not derived from the wire response — Greenhouse exposes
// only last-modified semantics (`updated_at`) and an unreliable `first_published`,
// neither of which match the domain meaning of "posted_at". The full job is still
// preserved verbatim in Posting.RawData for downstream re-interpretation.
type ghJob struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
}

// FetchPostings retrieves all jobs for the given Greenhouse board token and
// normalizes them into Postings. Any network, HTTP, or parse failure aborts
// the whole fetch — no partial successes. Field-level absence (empty string,
// missing key) is not an error; the corresponding Posting field is left nil.
func (g *Greenhouse) FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error) {
	escapedToken := url.PathEscape(boardToken)
	fetchURL := fmt.Sprintf("%s/%s/jobs?content=true", g.baseURL, escapedToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: building request for %s: %w", boardToken, err)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: fetching postings for %s: %w", boardToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes)) // body read error is non-actionable; status code is the signal
		return nil, fmt.Errorf("greenhouse: unexpected status %d for %s: %s", resp.StatusCode, boardToken, strconv.Quote(string(bytes.TrimSpace(snippet))))
	}

	// Read up to maxResponseBytes+1 so we can detect truncation by overflow:
	// reading exactly maxResponseBytes back is a legitimate response at the cap,
	// but maxResponseBytes+1 means the upstream had more to send.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("greenhouse: reading response body for %s: %w", boardToken, err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("greenhouse: response body for %s exceeded %d bytes (got %d)", boardToken, maxResponseBytes, len(body))
	}

	var wire ghResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("greenhouse: decoding response for %s: %w", boardToken, err)
	}

	postings := make([]domain.Posting, 0, len(wire.Jobs))
	for i, raw := range wire.Jobs {
		var job ghJob
		if err := json.Unmarshal(raw, &job); err != nil {
			return nil, fmt.Errorf("greenhouse: decoding job at index %d for %s: %w", i, boardToken, err)
		}

		if job.ID == 0 {
			return nil, fmt.Errorf("greenhouse: job at index %d for %s has missing or zero id", i, boardToken)
		}

		idStr := strconv.FormatInt(job.ID, 10) // decimal digits only — no URL escaping needed
		posting := domain.Posting{
			SourceID:  idStr,
			SourceURL: fmt.Sprintf("%s/%s/jobs/%s", g.baseURL, escapedToken, idStr),
			RawData:   raw,
		}

		posting.Title = ptrIfNonEmpty(job.Title)
		posting.JobURL = ptrIfNonEmpty(job.AbsoluteURL)
		posting.LocationText = ptrIfNonEmpty(job.Location.Name)
		if len(job.Departments) > 0 {
			posting.Department = ptrIfNonEmpty(job.Departments[0].Name)
		}

		postings = append(postings, posting)
	}

	return postings, nil
}

func ptrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
