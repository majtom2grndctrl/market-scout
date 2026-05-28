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

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/domain"
)

const workablePublicBaseURL = "https://apply.workable.com/api/v1/widget/accounts"

// workableSanityCeiling guards against an unexpectedly large widget response.
// The widget endpoint is single-shot (no pagination), so a response above this
// size suggests either Workable shipping pagination semantics we don't handle
// or a misuse of the endpoint. Log a warn at this threshold; do not fail.
const workableSanityCeiling = 1000

// Workable reads from the Workable public widget endpoint
// (https://apply.workable.com/api/v1/widget/accounts/{slug}). The endpoint
// requires no auth and returns all published jobs for an account in one
// response, like Greenhouse and Ashby.
type Workable struct {
	client  *http.Client
	baseURL string
}

// NewWorkable returns a Workable adapter. Nil client falls back to
// http.DefaultClient. No client-level timeout is set; cancellation flows
// through the context passed to FetchPostings.
func NewWorkable(client *http.Client) *Workable {
	return newWorkableWithBaseURL(client, workablePublicBaseURL)
}

// newWorkableWithBaseURL is test-only. Points the adapter at an arbitrary base
// URL (e.g. an httptest.Server) without mutating package-level state.
func newWorkableWithBaseURL(client *http.Client, baseURL string) *Workable {
	if client == nil {
		client = http.DefaultClient
	}
	return &Workable{client: client, baseURL: baseURL}
}

// wkResponse is the wire envelope. Per-job bytes are json.RawMessage so
// RawData preserves the original payload.
type wkResponse struct {
	Name string            `json:"name"`
	Jobs []json.RawMessage `json:"jobs"`
}

// wkJob is the subset of the per-job wire shape the adapter normalizes.
// The widget endpoint returns summary-level fields only — no description and
// no compensation — so those Posting fields stay nil. RawData preserves the
// full payload for later re-interpretation.
type wkJob struct {
	Title          string `json:"title"`
	Shortcode      string `json:"shortcode"`
	URL            string `json:"url"`
	ApplicationURL string `json:"application_url"`
	Department     string `json:"department"`
	EmploymentType string `json:"employment_type"`
	PublishedOn    string `json:"published_on"` // YYYY-MM-DD when present
	CreatedAt      string `json:"created_at"`   // RFC3339 when present
	Country        string `json:"country"`
	City           string `json:"city"`
	State          string `json:"state"`
}

// FetchPostings retrieves all jobs for the given Workable account slug and
// normalizes them into Postings. board_token is the lowercase slug from the
// company's apply.workable.com URL (e.g. "acme" for apply.workable.com/acme).
// Any network, HTTP, or parse failure aborts the whole fetch.
func (w *Workable) FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error) {
	token := strings.TrimSpace(boardToken)
	if token == "" {
		return nil, fmt.Errorf("workable: invalid board_token %q: empty token", boardToken)
	}
	if strings.ContainsAny(token, "/?#") || strings.Contains(token, "://") {
		return nil, fmt.Errorf("workable: invalid board_token %q: must be a bare slug", boardToken)
	}

	fetchURL := fmt.Sprintf("%s/%s", w.baseURL, url.PathEscape(token))

	body, err := httpFetch(ctx, w.client, fetchURL)
	if err != nil {
		return nil, fmt.Errorf("workable: fetching postings for %s: %w", boardToken, err)
	}

	var wire wkResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("workable: decoding response for %s: %w", boardToken, err)
	}

	if len(wire.Jobs) >= workableSanityCeiling {
		slog.Warn("[workable] response near sanity ceiling — Workable may have introduced pagination",
			"boardToken", boardToken, "count", len(wire.Jobs), "ceiling", workableSanityCeiling)
	}

	postings := make([]domain.Posting, 0, len(wire.Jobs))
	for i, raw := range wire.Jobs {
		posting, err := decodeWorkableJob(raw, boardToken, i)
		if err != nil {
			return nil, err
		}
		postings = append(postings, posting)
	}

	return postings, nil
}

func decodeWorkableJob(raw json.RawMessage, boardToken string, index int) (domain.Posting, error) {
	var job wkJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return domain.Posting{}, fmt.Errorf("workable: decoding job at index %d for %s: %w", index, boardToken, err)
	}

	if job.Shortcode == "" {
		return domain.Posting{}, fmt.Errorf("workable: job at index %d for %s has missing shortcode", index, boardToken)
	}

	// Prefer the wire's url (canonical apply page); fall back to application_url.
	// SourceURL is required — an empty resolution is a contract violation.
	resolvedURL := job.URL
	if resolvedURL == "" {
		resolvedURL = job.ApplicationURL
	}
	if resolvedURL == "" {
		return domain.Posting{}, fmt.Errorf("workable: job at index %d for %s (shortcode=%s) has no url or application_url", index, boardToken, job.Shortcode)
	}

	posting := domain.Posting{
		SourceID:  job.Shortcode,
		SourceURL: resolvedURL,
		JobURL:    ptrIfNonEmpty(resolvedURL),
		RawData:   raw,
	}

	posting.Title = ptrIfNonEmpty(job.Title)
	posting.Department = ptrIfNonEmpty(job.Department)
	posting.EmploymentType = normalizeWorkableEmploymentType(job.EmploymentType)

	// Location is flat (city/state/country) in the widget response. Join
	// non-empty parts with ", ". Wrap in 1-element LocationTexts for parity
	// with other adapters; nil when no location parts present.
	if loc := joinNonEmpty(", ", job.City, job.State, job.Country); loc != "" {
		posting.LocationText = &loc
		posting.LocationTexts = []string{loc}
	}

	// published_on is YYYY-MM-DD in the account's local calendar. Treat as
	// midnight UTC — timezone isn't on the wire and PostedAt is day-resolution
	// only. Unparseable values warn and leave PostedAt nil; one bad date does
	// not abort the whole fetch (matches Workday).
	if job.PublishedOn != "" {
		t, err := time.Parse("2006-01-02", job.PublishedOn)
		if err != nil {
			slog.Warn("[workable] unparseable published_on", "value", job.PublishedOn, "boardToken", boardToken, "shortcode", job.Shortcode)
		} else {
			t = t.UTC()
			posting.PostedAt = &t
		}
	}

	// created_at varies across Workable boards: some return RFC3339
	// (2024-04-25T10:30:00Z), others return date-only (2024-04-25). Try both
	// rather than picking one and warning on the other shape.
	// Same lenient policy as published_on: warn and leave nil on parse error.
	if job.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, job.CreatedAt)
		if err != nil {
			t, err = time.Parse("2006-01-02", job.CreatedAt)
		}
		if err != nil {
			slog.Warn("[workable] unparseable created_at", "value", job.CreatedAt, "boardToken", boardToken, "shortcode", job.Shortcode)
		} else {
			t = t.UTC()
			posting.SourceFirstPublishedAt = &t
		}
	}

	return posting, nil
}

// workableEmploymentAliases maps Workable employment-type wire values (after
// lowercase + strip-non-alphanum normalization) to schema enum values.
// Workable's documented enum: Full-time, Part-time, Contract, Intern, Temporary.
var workableEmploymentAliases = map[string]string{
	"fulltime":  "full_time",
	"parttime":  "part_time",
	"contract":  "contract",
	"intern":    "intern",
	"temporary": "temporary",
}

// normalizeWorkableEmploymentType maps Workable's employment_type wire value to
// the schema enum. Unknown values return nil and emit a [workable] warn.
func normalizeWorkableEmploymentType(raw string) *string {
	if raw == "" {
		return nil
	}
	key := stripNonAlphaNumLower(raw)
	if key == "" {
		return nil
	}
	if v, ok := workableEmploymentAliases[key]; ok {
		return &v
	}
	slog.Warn("[workable] unknown employment_type", "value", raw)
	return nil
}

// joinNonEmpty joins parts with sep, dropping empty strings.
func joinNonEmpty(sep string, parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
