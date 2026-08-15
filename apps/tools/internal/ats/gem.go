package ats

import (
	"bytes"
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

const gemPublicBaseURL = "https://api.gem.com/job_board/v0"

// gemSanityCeiling guards against an unexpectedly large list response. Gem's
// endpoint is single-shot (no pagination), so a response at this threshold may
// mean Gem has introduced pagination semantics the adapter does not handle.
// Warn, rather than fail, so a board remains ingestible while that is checked.
const gemSanityCeiling = 1000

// Gem reads the public Job Board API
// (https://api.gem.com/job_board/v0/{slug}/job_posts/). The endpoint requires
// no auth and returns every published job as a bare JSON array in one response.
// Its list records include full descriptions, unlike the Workday and Workable
// listing APIs.
type Gem struct {
	client  *http.Client
	baseURL string
}

// NewGem returns a Gem adapter. Nil client falls back to http.DefaultClient.
// No client-level timeout is set; cancellation flows through FetchPostings.
func NewGem(client *http.Client) *Gem {
	return newGemWithBaseURL(client, gemPublicBaseURL)
}

// newGemWithBaseURL is test-only. It points the adapter at an arbitrary base
// URL without mutating package-level state.
func newGemWithBaseURL(client *http.Client, baseURL string) *Gem {
	if client == nil {
		client = http.DefaultClient
	}
	return &Gem{client: client, baseURL: baseURL}
}

// gemJob is the subset of Gem's list-record shape normalized into Posting.
// Per-job raw bytes remain in RawData so all unmodeled fields, including
// created_at, internal_job_id, requisition_id, and full office metadata, are
// available for later interpretation without another fetch.
type gemJob struct {
	ID               string `json:"id"`
	AbsoluteURL      string `json:"absolute_url"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	ContentPlain     string `json:"content_plain"`
	FirstPublishedAt string `json:"first_published_at"`
	UpdatedAt        string `json:"updated_at"`
	Location         struct {
		Name string `json:"name"`
	} `json:"location"`
	Offices []struct {
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"offices"`
	LocationType   string `json:"location_type"`
	EmploymentType string `json:"employment_type"`
	Departments    []struct {
		Name string `json:"name"`
	} `json:"departments"`
}

// gemWorkplaceAliases maps Gem's observed location_type values to the schema
// vocabulary. in_office is a wire-side spelling; the database enum calls the
// same state onsite.
var gemWorkplaceAliases = map[string]string{
	"hybrid":    "hybrid",
	"remote":    "remote",
	"in_office": "onsite",
}

// gemEmploymentAliases maps normalized employment_type values to the schema
// enum. full_time is the only value observed on Gem boards; the other four
// entries are speculative aliases retained for the documented schema values.
var gemEmploymentAliases = map[string]string{
	"fulltime":  "full_time",
	"parttime":  "part_time",
	"contract":  "contract",
	"intern":    "intern",
	"temporary": "temporary",
}

// FetchPostings retrieves every posting for a Gem board and normalizes it into
// Postings. Any network, HTTP, or parse failure aborts the whole fetch.
func (g *Gem) FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error) {
	token := strings.TrimSpace(boardToken)
	if token == "" {
		return nil, fmt.Errorf("gem: invalid board_token %q: empty token", boardToken)
	}
	if strings.ContainsAny(token, "/?#") || strings.Contains(token, "://") {
		return nil, fmt.Errorf("gem: invalid board_token %q: must be a bare slug", boardToken)
	}

	fetchURL := fmt.Sprintf("%s/%s/job_posts/", g.baseURL, url.PathEscape(token))
	body, err := httpFetch(ctx, g.client, fetchURL)
	if err != nil {
		return nil, fmt.Errorf("gem: fetching postings for %s: %w", boardToken, err)
	}

	var response json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("gem: decoding response for %s: %w", boardToken, err)
	}
	if response = bytes.TrimSpace(response); len(response) == 0 || response[0] != '[' {
		return nil, fmt.Errorf("gem: decoding response for %s: expected bare JSON array", boardToken)
	}
	var rawJobs []json.RawMessage
	if err := json.Unmarshal(response, &rawJobs); err != nil {
		return nil, fmt.Errorf("gem: decoding response for %s: %w", boardToken, err)
	}
	if len(rawJobs) >= gemSanityCeiling {
		slog.Warn("[gem] response near sanity ceiling — Gem may have introduced pagination",
			"boardToken", boardToken, "count", len(rawJobs), "ceiling", gemSanityCeiling)
	}

	postings := make([]domain.Posting, 0, len(rawJobs))
	for i, raw := range rawJobs {
		posting, err := decodeGemJob(raw, boardToken, i)
		if err != nil {
			return nil, err
		}
		postings = append(postings, posting)
	}
	return postings, nil
}

func decodeGemJob(raw json.RawMessage, boardToken string, index int) (domain.Posting, error) {
	var job gemJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return domain.Posting{}, fmt.Errorf("gem: decoding job at index %d for %s: %w", index, boardToken, err)
	}
	if job.ID == "" {
		return domain.Posting{}, fmt.Errorf("gem: job at index %d for %s has missing id", index, boardToken)
	}
	if job.AbsoluteURL == "" {
		return domain.Posting{}, fmt.Errorf("gem: job at index %d for %s has missing absolute_url", index, boardToken)
	}

	posting := domain.Posting{
		SourceID:  job.ID,
		SourceURL: job.AbsoluteURL,
		JobURL:    ptrIfNonEmpty(job.AbsoluteURL),
		RawData:   raw,
	}
	posting.Title = ptrIfNonEmpty(job.Title)
	if len(job.Departments) > 0 {
		posting.Department = ptrIfNonEmpty(job.Departments[0].Name)
	}

	// Office labels can combine a city and modality (for example, "Seattle
	// Hybrid"), so only explicit office.location.name values are locations.
	// Empty or label-only offices add no office-derived location. The top-level
	// location.name is the fallback when present; both fields stay nil only when
	// neither supplies a place name.
	for _, office := range job.Offices {
		if office.Location.Name != "" {
			posting.LocationTexts = append(posting.LocationTexts, office.Location.Name)
		}
	}
	if len(posting.LocationTexts) == 0 && job.Location.Name != "" {
		posting.LocationTexts = []string{job.Location.Name}
	}
	if len(posting.LocationTexts) > 0 {
		posting.LocationText = ptrIfNonEmpty(posting.LocationTexts[0])
	}

	posting.WorkplaceType = normalizeGemWorkplaceType(job.LocationType)
	posting.EmploymentType = normalizeGemEmploymentType(job.EmploymentType)
	if job.ContentPlain != "" {
		posting.DescriptionText = ptrIfNonEmpty(job.ContentPlain)
	} else if text := htmlToPlainText(job.Content); text != "" {
		posting.DescriptionText = &text
	}

	if job.FirstPublishedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, job.FirstPublishedAt)
		if err != nil {
			return domain.Posting{}, fmt.Errorf("gem: job at index %d for %s: parse first_published_at %q: %w", index, boardToken, job.FirstPublishedAt, err)
		}
		posting.PostedAt = &t
		posting.SourceFirstPublishedAt = &t
	}
	if job.UpdatedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, job.UpdatedAt)
		if err != nil {
			return domain.Posting{}, fmt.Errorf("gem: job at index %d for %s: parse updated_at %q: %w", index, boardToken, job.UpdatedAt, err)
		}
		posting.SourceLastModifiedAt = &t
	}

	return posting, nil
}

func normalizeGemWorkplaceType(raw string) *string {
	if raw == "" {
		return nil
	}
	if value, ok := gemWorkplaceAliases[strings.ToLower(raw)]; ok {
		return &value
	}
	slog.Warn("[gem] unknown location_type", "value", raw)
	return nil
}

func normalizeGemEmploymentType(raw string) *string {
	if raw == "" {
		return nil
	}
	key := stripNonAlphaNumLower(raw)
	if key == "" {
		return nil
	}
	if value, ok := gemEmploymentAliases[key]; ok {
		return &value
	}
	slog.Warn("[gem] unknown employment_type", "value", raw)
	return nil
}
