package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

const workdayPageSize = 20

// Workday reads from the Workday CXS public job board API.
//
// Each tenant runs its own host (e.g. nvidia.wd5.myworkdayjobs.com) with one
// or more named sites beneath it, so the request URL is composed from a
// {host}/{site} board_token rather than a package-level base URL constant.
// baseURL is empty in production; tests set it to an httptest.Server URL.
// The parsed host is used only for the fallback SourceURL.
type Workday struct {
	client  *http.Client
	baseURL string // empty = use https://{host} parsed from boardToken; set for tests
}

// NewWorkday returns a Workday adapter. Nil client falls back to
// http.DefaultClient. No client-level timeout is set; cancellation flows
// through the context passed to FetchPostings.
func NewWorkday(client *http.Client) *Workday {
	return newWorkdayWithBaseURL(client, "")
}

// newWorkdayWithBaseURL is test-only. Points the adapter at an arbitrary base
// URL (e.g. an httptest.Server) without mutating package-level state.
func newWorkdayWithBaseURL(client *http.Client, baseURL string) *Workday {
	if client == nil {
		client = http.DefaultClient
	}
	return &Workday{client: client, baseURL: baseURL}
}

// wdRequest is the POST body for the Workday CXS jobs endpoint. AppliedFacets
// must be present — the API rejects requests that omit the key. An empty
// object means no filters.
type wdRequest struct {
	Limit         int            `json:"limit"`
	Offset        int            `json:"offset"`
	SearchText    string         `json:"searchText"`
	AppliedFacets map[string]any `json:"appliedFacets"`
}

// wdResponse is the wire envelope. Per-job bytes are json.RawMessage so
// RawData preserves the original payload.
type wdResponse struct {
	Total       int               `json:"total"`
	JobPostings []json.RawMessage `json:"jobPostings"`
}

// wdJob is the subset of the per-job wire shape the adapter normalizes.
// The CXS list endpoint exposes no description, compensation, or last-modified
// timestamp — those Posting fields stay nil. RawData preserves the full
// payload for later re-interpretation.
type wdJob struct {
	Title         string   `json:"title"`
	ExternalPath  string   `json:"externalPath"`
	LocationsText string   `json:"locationsText"`
	PostedOn      string   `json:"postedOn"`
	ExternalURL   string   `json:"externalUrl"`
	BulletFields  []string `json:"bulletFields"`
}

// FetchPostings retrieves all jobs for the given board_token and normalizes
// them into Postings. board_token is {host}/{site} (e.g.
// nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite). Parsed on every
// call because adapters are shared across boards. Any failure aborts the
// whole fetch.
func (w *Workday) FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error) {
	host, site, tenant, err := parseWorkdayBoardToken(boardToken)
	if err != nil {
		return nil, fmt.Errorf("workday: invalid board_token %q: %w", boardToken, err)
	}

	// hostBase is for the fallback SourceURL only — always the real tenant
	// host, even when w.baseURL redirects API requests to a test server.
	hostBase := "https://" + host
	apiBase := hostBase
	if w.baseURL != "" {
		apiBase = w.baseURL
	}
	fetchURL := fmt.Sprintf("%s/wday/cxs/%s/%s/jobs", apiBase, tenant, site)

	var postings []domain.Posting
	offset := 0
	// Sanity ceiling: 1000 pages × workdayPageSize = 20k jobs. Guards against
	// upstreams that misreport total or never advance offset.
	const maxPages = 1000
	const maxPagesWarnAt = 500
	for page := 0; ; page++ {
		if page >= maxPages {
			return nil, fmt.Errorf("workday: pagination exceeded sanity ceiling for %s", boardToken)
		}
		if page == maxPagesWarnAt {
			slog.Warn("[workday] pagination approaching sanity ceiling", "boardToken", boardToken, "page", page, "maxPages", maxPages)
		}

		reqBody, err := json.Marshal(wdRequest{
			Limit:         workdayPageSize,
			Offset:        offset,
			SearchText:    "",
			AppliedFacets: map[string]any{},
		})
		if err != nil {
			return nil, fmt.Errorf("workday: encoding request for %s: %w", boardToken, err)
		}

		body, err := httpPost(ctx, w.client, fetchURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("workday: fetching postings for %s: %w", boardToken, err)
		}

		var wire wdResponse
		if err := json.Unmarshal(body, &wire); err != nil {
			return nil, fmt.Errorf("workday: decoding response for %s: %w", boardToken, err)
		}

		for i, raw := range wire.JobPostings {
			posting, err := decodeWorkdayJob(raw, boardToken, host, site, offset+i)
			if err != nil {
				return nil, err
			}
			postings = append(postings, posting)
		}

		if len(wire.JobPostings) == 0 {
			break
		}
		offset += len(wire.JobPostings)
		if offset >= wire.Total || len(wire.JobPostings) < workdayPageSize {
			break
		}
	}

	return postings, nil
}

// parseWorkdayBoardToken splits a {host}/{site} board_token into host, site,
// and tenant (first DNS label of host). Rejects tokens with a scheme, leading
// or trailing slash, or any missing component.
func parseWorkdayBoardToken(boardToken string) (host, site, tenant string, err error) {
	tok := strings.TrimSpace(boardToken)
	if tok == "" {
		return "", "", "", fmt.Errorf("empty token")
	}
	if strings.Contains(tok, "://") {
		return "", "", "", fmt.Errorf("must not contain scheme")
	}
	if strings.HasPrefix(tok, "/") || strings.HasSuffix(tok, "/") {
		return "", "", "", fmt.Errorf("must not start or end with %q", "/")
	}
	slash := strings.Index(tok, "/")
	if slash < 0 {
		return "", "", "", fmt.Errorf("expected {host}/{site}")
	}
	host = tok[:slash]
	site = tok[slash+1:]
	if host == "" {
		return "", "", "", fmt.Errorf("empty host")
	}
	if site == "" {
		return "", "", "", fmt.Errorf("empty site")
	}
	dot := strings.Index(host, ".")
	if dot <= 0 {
		return "", "", "", fmt.Errorf("host missing tenant label")
	}
	tenant = host[:dot]
	if tenant == "" {
		return "", "", "", fmt.Errorf("empty tenant")
	}
	return host, site, tenant, nil
}

func decodeWorkdayJob(raw json.RawMessage, boardToken, host, site string, index int) (domain.Posting, error) {
	var job wdJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return domain.Posting{}, fmt.Errorf("workday: decoding job at index %d for %s: %w", index, boardToken, err)
	}

	if job.ExternalPath == "" {
		return domain.Posting{}, fmt.Errorf("workday: job at index %d for %s has missing externalPath", index, boardToken)
	}

	// Prefer the wire's externalUrl; fall back to composing from host + site +
	// externalPath. externalPath often has a leading slash — strip it to avoid
	// a double-slash in the composed URL.
	resolvedURL := job.ExternalURL
	if resolvedURL == "" {
		resolvedURL = fmt.Sprintf("https://%s/en-US/%s/%s", host, site, strings.TrimPrefix(job.ExternalPath, "/"))
	}

	posting := domain.Posting{
		SourceID:  job.ExternalPath,
		SourceURL: resolvedURL,
		JobURL:    ptrIfNonEmpty(resolvedURL),
		RawData:   raw,
	}

	posting.Title = ptrIfNonEmpty(job.Title)

	// CXS list endpoint returns a single rendered string, not a structured
	// array. Wrap verbatim for parity with other adapters' LocationTexts.
	if job.LocationsText != "" {
		loc := job.LocationsText
		posting.LocationText = &loc
		posting.LocationTexts = []string{job.LocationsText}
	}

	// postedOn is YYYY-MM-DD in the tenant's local calendar. Treat as midnight
	// UTC: timezone is not on the wire and PostedAt is day-resolution only, so
	// sub-day skew is noise. Unparseable values warn and leave both timestamp
	// fields nil — one bad date should not abort the whole fetch.
	if job.PostedOn != "" {
		t, err := time.Parse("2006-01-02", job.PostedOn)
		if err != nil {
			slog.Warn("[workday] unparseable postedOn", "value", job.PostedOn, "boardToken", boardToken, "externalPath", job.ExternalPath)
		} else {
			t = t.UTC()
			posting.PostedAt = &t
			posting.SourceFirstPublishedAt = &t
		}
	}

	return posting, nil
}
