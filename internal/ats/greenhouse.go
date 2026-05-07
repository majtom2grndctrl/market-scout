// Package ats hosts ATS adapter implementations: Greenhouse, Lever, and Ashby.
// Adapters return domain.Posting from internal/domain.
// The consumer-side interface (atsAdapter) lives in cmd/fetcher — concrete
// adapters satisfy it implicitly via Go structural typing.
// See: agent-context/lib/project.md
package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

const greenhousePublicBaseURL = "https://boards-api.greenhouse.io/v1/boards"

// Greenhouse reads from the Greenhouse public Job Board API.
// The API requires no auth and returns all jobs for a board in a single request.
type Greenhouse struct {
	client  *http.Client
	baseURL string
}

// NewGreenhouse returns a Greenhouse adapter. If client is nil, http.DefaultClient is used.
// The adapter does not set a client-level timeout; cancellation flows from the context
// passed to FetchPostings.
func NewGreenhouse(client *http.Client) *Greenhouse {
	return newGreenhouseWithBaseURL(client, greenhousePublicBaseURL)
}

// newGreenhouseWithBaseURL constructs a Greenhouse adapter pointed at an arbitrary base URL.
// Test-only: lets adapter tests target an httptest.Server without mutating
// package-level state. Production callers use NewGreenhouse.
func newGreenhouseWithBaseURL(client *http.Client, baseURL string) *Greenhouse {
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

// Compensation spike (2026-05): surveyed all five seeded Greenhouse boards
// (anthropic, stripe, figma, scaleai, gleanwork) via the public Job Board API
// with `?content=true`. None expose a top-level `pay_input_ranges` field, and
// none expose a `metadata` entry with `value_type: "currency_range"`,
// `"currency"`, or any name matching pay/salary/compensation. The only metadata
// seen in the sample is non-comp (e.g. Anthropic's "Location Type"
// single_select). Per-job keys returned today:
//   absolute_url, application_deadline, company_name, content, data_compliance,
//   departments, first_published, id, internal_job_id, language, location,
//   metadata, offices, requisition_id, title, updated_at
// Where compensation does appear (Figma, some Stripe roles), it is embedded in
// the rendered `content` HTML — e.g. <div class="pay-range">$165,000 — $190,000 USD</div>
// — not in any structured field. Structured Greenhouse compensation parsing is
// not viable against the current watchlist; the only signal lives in the
// description body. Lever is the structured compensation path; Ashby defers
// (API exposes summary strings only).

// ghJob is the subset of the per-job wire shape the adapter extracts into Posting.
// FirstPublished and UpdatedAt are captured verbatim into SourceFirstPublishedAt
// and SourceLastModifiedAt. Neither is promoted to PostedAt: `updated_at` is
// last-modified semantics (not posting age), and `first_published` is unreliable
// across Greenhouse boards. The full job is preserved in RawData for re-interpretation.
type ghJob struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	AbsoluteURL    string `json:"absolute_url"`
	FirstPublished string `json:"first_published"`
	UpdatedAt      string `json:"updated_at"`
	Content        string `json:"content"`
	Location       struct {
		Name string `json:"name"`
	} `json:"location"`
	Departments []struct {
		Name string `json:"name"`
	} `json:"departments"`
	// PayInputRanges: 2026-05 spike found no live watchlist board exposes
	// structured compensation here. Captured as raw messages so the parsing
	// path ships dormant — when a board eventually surfaces it, the shape
	// can be inspected and a typed sub-struct added without re-fetching.
	PayInputRanges []json.RawMessage `json:"pay_input_ranges"`
}

// FetchPostings retrieves all jobs for the given Greenhouse board token and
// normalizes them into Postings. Any network, HTTP, or parse failure aborts
// the whole fetch — no partial successes. Field-level absence (empty string,
// missing key) is not an error; the corresponding Posting field is left nil.
func (g *Greenhouse) FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error) {
	escapedToken := url.PathEscape(boardToken)
	fetchURL := fmt.Sprintf("%s/%s/jobs?content=true", g.baseURL, escapedToken)

	body, err := httpFetch(ctx, g.client, fetchURL)
	if err != nil {
		return nil, fmt.Errorf("greenhouse: fetching postings for %s: %w", boardToken, err)
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
		// why: LocationTexts is the multi-source array column; Greenhouse exposes
		// a single location string. Wrap it in a slice so Greenhouse rows have
		// parity with Lever/Ashby rows and no NULL skew. Nil when empty — never
		// an empty slice — so absence is unambiguous.
		if job.Location.Name != "" {
			posting.LocationTexts = []string{job.Location.Name}
		}
		if len(job.Departments) > 0 {
			posting.Department = ptrIfNonEmpty(job.Departments[0].Name)
		}

		if text := htmlToPlainText(job.Content); text != "" {
			posting.DescriptionText = &text
		}

		// Compensation: 2026-05 spike confirmed no current Greenhouse board
		// exposes pay_input_ranges. The pathway ships dormant — when the field
		// arrives on a future board, inspect a sample, define a typed
		// sub-struct, and parse pay_input_ranges[0] here. For now, leaving all
		// four comp fields nil is correct: an empty PayInputRanges yields NULL.

		if job.FirstPublished != "" {
			t, err := time.Parse(time.RFC3339Nano, job.FirstPublished)
			if err != nil {
				return nil, fmt.Errorf("greenhouse: job at index %d for %s: parse %s %q: %w", i, boardToken, "first_published", job.FirstPublished, err)
			}
			posting.SourceFirstPublishedAt = &t
		}
		if job.UpdatedAt != "" {
			t, err := time.Parse(time.RFC3339Nano, job.UpdatedAt)
			if err != nil {
				return nil, fmt.Errorf("greenhouse: job at index %d for %s: parse %s %q: %w", i, boardToken, "updated_at", job.UpdatedAt, err)
			}
			posting.SourceLastModifiedAt = &t
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
