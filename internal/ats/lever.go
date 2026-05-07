package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

const (
	leverPublicBaseURL = "https://api.lever.co/v0/postings"
	leverPageSize      = 100
)

// Lever reads from the Lever public postings API.
// The API requires no auth. Live-API findings against
// `https://api.lever.co/v0/postings/leverdemo?mode=json`:
//   - the response is a bare JSON array of jobs (no envelope, no
//     `next` / `hasNext` field).
//   - `?limit=100` caps the response size; `?skip=N` pages forward by
//     index. (`offset=N` is silently ignored on this API.)
//   - termination signal is "fewer than limit results returned."
type Lever struct {
	client  *http.Client
	baseURL string
}

// NewLever returns a Lever adapter. If client is nil, http.DefaultClient is used.
// The adapter does not set a client-level timeout; cancellation flows from the
// context passed to FetchPostings.
func NewLever(client *http.Client) *Lever {
	return newLeverWithBaseURL(client, leverPublicBaseURL)
}

// newLeverWithBaseURL constructs a Lever adapter pointed at an arbitrary base URL.
// Test-only: lets adapter tests target an httptest.Server without mutating
// package-level state. Production callers use NewLever.
func newLeverWithBaseURL(client *http.Client, baseURL string) *Lever {
	if client == nil {
		client = http.DefaultClient
	}
	return &Lever{client: client, baseURL: baseURL}
}

// leverJob is the subset of the per-job wire shape the adapter extracts into
// Posting. The full job is preserved in RawData so fields not declared here
// (description HTML, lists, additional, salaryRange, opening, etc.) can be
// re-interpreted later without re-fetching.
//
// `categories.allLocations` is the multi-market field; `categories.location`
// is the single-string fallback. `createdAt` is a millisecond epoch and is
// the only date Lever exposes — there is no separate updated/modified field,
// so SourceLastModifiedAt is always left nil. The same `createdAt` value
// populates both PostedAt (for trend queries) and SourceFirstPublishedAt
// (for change detection): Lever doesn't distinguish creation from
// first-publication, so the single timestamp is the best signal for both.
// `workplaceType` is lowercase wire (`onsite | hybrid | remote | unspecified`).
// `categories.commitment` is free-text employment type, normalized at the
// adapter boundary.
//
// AllLocations is `*[]string` so the adapter can distinguish wire-absent
// (nil) from explicit empty array (non-nil, len 0); the distinction is
// load-bearing for domain.Posting.LocationTexts.
type leverJob struct {
	ID            string `json:"id"`
	Text          string `json:"text"` // Lever's wire field for the job title.
	HostedURL     string `json:"hostedUrl"`
	CreatedAt     int64  `json:"createdAt"`
	WorkplaceType string `json:"workplaceType"`
	Categories    struct {
		Team         string    `json:"team"`
		Department   string    `json:"department"`
		Commitment   string    `json:"commitment"`
		Location     string    `json:"location"`
		AllLocations *[]string `json:"allLocations"`
	} `json:"categories"`
}

// leverWorkplaceAliases maps Lever's wire workplaceType values to the
// schema enum. Naming the schema-side vocabulary in one place keeps the
// adapter robust to wire-side casing drift: an unrecognized key (including
// any future casing change) returns nil + a [lever] warn rather than
// silently propagating a Go-local lowercased string.
var leverWorkplaceAliases = map[string]string{
	"onsite": "onsite",
	"hybrid": "hybrid",
	"remote": "remote",
}

// leverEmploymentAliases maps normalized commitment strings (lowercased,
// non-alphanumerics stripped) to schema enum values. Extend by adding
// observed wire values surfaced through the [lever] unknown-commitment
// warn channel.
var leverEmploymentAliases = map[string]string{
	"fulltime":   "full_time",
	"ft":         "full_time",
	"parttime":   "part_time",
	"pt":         "part_time",
	"contract":   "contract",
	"contractor": "contract",
	"intern":     "intern",
	"internship": "intern",
	"temporary":  "temporary",
	"temp":       "temporary",
}

// FetchPostings retrieves all jobs for the given Lever board token and
// normalizes them into Postings. Pagination uses `skip` to advance through
// pages of `leverPageSize`; the loop terminates when the API returns fewer
// rows than the requested limit (no `hasNext` field exists on this API).
// Any network, HTTP, or parse failure aborts the whole fetch — no partial
// successes.
func (l *Lever) FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error) {
	escapedToken := url.PathEscape(boardToken)

	var postings []domain.Posting
	skip := 0
	// Sanity ceiling: 1000 pages = 100k jobs. Guards against an upstream that
	// keeps returning full pages forever (the only loop-exit signal is a
	// short page). Surfaces as a wrapped error at the fetcher boundary
	// rather than silently OOM-ing.
	const maxPages = 1000
	const maxPagesWarnAt = 500
	for page := 0; ; page++ {
		if page >= maxPages {
			return nil, fmt.Errorf("lever: pagination exceeded sanity ceiling for %s", boardToken)
		}
		// Single early-warning breadcrumb halfway to the ceiling. Surfaces a
		// runaway pagination loop in logs before it errors out, without
		// per-page spam.
		if page == maxPagesWarnAt {
			slog.Warn("[lever] pagination approaching sanity ceiling", "boardToken", boardToken, "page", page, "maxPages", maxPages)
		}
		fetchURL := fmt.Sprintf("%s/%s?limit=%d&skip=%d&mode=json", l.baseURL, escapedToken, leverPageSize, skip)

		body, err := httpFetch(ctx, l.client, fetchURL)
		if err != nil {
			return nil, fmt.Errorf("lever: fetching postings for %s: %w", boardToken, err)
		}

		// The Lever public postings API returns a bare JSON array; per-job raw
		// bytes are preserved via json.RawMessage so RawData carries the original.
		var rawJobs []json.RawMessage
		if err := json.Unmarshal(body, &rawJobs); err != nil {
			return nil, fmt.Errorf("lever: decoding response for %s: %w", boardToken, err)
		}

		for i, raw := range rawJobs {
			// skip+i is the absolute index across pages (not just within this page);
			// used for stable error messages when a single bad job aborts the fetch.
			posting, err := decodeLeverJob(raw, boardToken, skip+i)
			if err != nil {
				return nil, err
			}
			postings = append(postings, posting)
		}

		if len(rawJobs) < leverPageSize {
			break
		}
		skip += leverPageSize
	}

	return postings, nil
}

func decodeLeverJob(raw json.RawMessage, boardToken string, index int) (domain.Posting, error) {
	var job leverJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return domain.Posting{}, fmt.Errorf("lever: decoding job at index %d for %s: %w", index, boardToken, err)
	}

	if job.ID == "" {
		return domain.Posting{}, fmt.Errorf("lever: job at index %d for %s has missing id", index, boardToken)
	}
	if job.HostedURL == "" {
		return domain.Posting{}, fmt.Errorf("lever: job %s for %s has missing hostedUrl", job.ID, boardToken)
	}

	posting := domain.Posting{
		SourceID:  job.ID,
		SourceURL: job.HostedURL,
		JobURL:    ptrIfNonEmpty(job.HostedURL),
		RawData:   raw,
	}

	posting.Title = ptrIfNonEmpty(job.Text)
	posting.Team = ptrIfNonEmpty(job.Categories.Team)
	posting.Department = ptrIfNonEmpty(job.Categories.Department)

	// LocationTexts: prefer allLocations; fall back to [location]; nil otherwise.
	// Keep the source strings verbatim — no rendering.
	// Honor the nil-vs-empty distinction on allLocations: an explicit `[]` on
	// the wire becomes a non-nil empty slice (per domain.Posting.LocationTexts
	// contract), while a missing field falls back to [location].
	switch {
	case job.Categories.AllLocations != nil && len(*job.Categories.AllLocations) > 0:
		posting.LocationTexts = *job.Categories.AllLocations
	case job.Categories.AllLocations != nil:
		posting.LocationTexts = []string{}
	case job.Categories.Location != "":
		posting.LocationTexts = []string{job.Categories.Location}
	}
	if len(posting.LocationTexts) > 0 {
		first := posting.LocationTexts[0]
		posting.LocationText = &first
	}

	posting.WorkplaceType = normalizeLeverWorkplaceType(job.WorkplaceType)
	posting.EmploymentType = normalizeLeverEmploymentType(job.Categories.Commitment)

	if job.CreatedAt != 0 {
		t := time.UnixMilli(job.CreatedAt).UTC()
		posting.PostedAt = &t
		posting.SourceFirstPublishedAt = &t
	}

	return posting, nil
}

// normalizeLeverWorkplaceType maps Lever's wire `workplaceType` to the schema
// enum via leverWorkplaceAliases. Lever's `unspecified` and any unrecognized
// value return nil; unrecognized values also emit a [lever] warn naming the
// wire string so genuinely-new values surface without aborting the fetch.
func normalizeLeverWorkplaceType(raw string) *string {
	if raw == "" {
		return nil
	}
	key := strings.ToLower(raw)
	if key == "unspecified" {
		return nil
	}
	if v, ok := leverWorkplaceAliases[key]; ok {
		return &v
	}
	slog.Warn("[lever] unknown workplaceType", "value", raw)
	return nil
}

// normalizeLeverEmploymentType maps Lever's free-text `categories.commitment`
// to the schema enum via lowercase + strip non-alphanumeric + alias-map
// lookup. Unknown values return nil and emit a [lever] warn naming the wire
// string. The starter alias map (leverEmploymentAliases) is extended as new
// wire values surface through the warn channel.
func normalizeLeverEmploymentType(raw string) *string {
	if raw == "" {
		return nil
	}
	key := stripNonAlphaNumLower(raw)
	if key == "" {
		return nil
	}
	if v, ok := leverEmploymentAliases[key]; ok {
		return &v
	}
	slog.Warn("[lever] unknown commitment", "value", raw)
	return nil
}

// stripNonAlphaNumLower lowercases s and drops every byte that is not an
// ASCII letter or digit. Shared with Ashby for employment-type normalization
// so free-text wire values (e.g. "Full-Time", "full time") collapse to a
// stable alias-map key.
func stripNonAlphaNumLower(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}
