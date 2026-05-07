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

const ashbyPublicBaseURL = "https://api.ashbyhq.com/posting-api/job-board"

// Ashby reads from the Ashby public Job Board posting API.
// The API requires no auth and returns all jobs for a clientname in a single
// request (no pagination).
//
// Wire shape pinned by the Task 6 live-API spike against several public boards
// (`ashby`, `posthog`, `linear`, `notion`):
//   - Top-level shape: `{"jobs": [...], "apiVersion": "..."}`.
//   - `workplaceType` arrives PascalCase (`OnSite | Hybrid | Remote`) and is
//     occasionally null. Lowercase + match against the enum handles all cases.
//   - `employmentType` is the documented enum: `FullTime | PartTime | Contract |
//     Intern | Temporary`.
//   - `publishedAt` is RFC 3339 with millisecond precision and a timezone offset
//     (e.g. `2024-03-04T14:29:08.532+00:00`); RFC3339Nano parses it.
//   - `department` and `team` are flat strings (not nested objects).
//   - No `updatedAt` field exists on the public API; SourceLastModifiedAt stays nil.
type Ashby struct {
	client  *http.Client
	baseURL string
}

// NewAshby returns an Ashby adapter. If client is nil, http.DefaultClient is used.
// The adapter does not set a client-level timeout; cancellation flows from the
// context passed to FetchPostings.
func NewAshby(client *http.Client) *Ashby {
	return newAshbyWithBaseURL(client, ashbyPublicBaseURL)
}

// newAshbyWithBaseURL constructs an Ashby adapter pointed at an arbitrary base URL.
// Test-only: lets adapter tests target an httptest.Server without mutating
// package-level state. Production callers use NewAshby.
func newAshbyWithBaseURL(client *http.Client, baseURL string) *Ashby {
	if client == nil {
		client = http.DefaultClient
	}
	return &Ashby{client: client, baseURL: baseURL}
}

// ashbyResponse mirrors the wire-level envelope. Per-job bytes are kept as
// json.RawMessage so RawData carries the original payload.
type ashbyResponse struct {
	Jobs []json.RawMessage `json:"jobs"`
}

// ashbyJob is the subset of the per-job wire shape the adapter normalizes.
// The full job is preserved in RawData; fields not declared here
// (descriptionHtml, descriptionPlain, applyUrl, isRemote, isListed,
// compensation, etc.) can be re-interpreted later without re-fetching.
type ashbyJob struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title"`
	Department         string              `json:"department"`
	Team               string              `json:"team"`
	EmploymentType     string              `json:"employmentType"`
	Location           string              `json:"location"`
	SecondaryLocations []ashbySecondaryLoc `json:"secondaryLocations"`
	PublishedAt        string              `json:"publishedAt"`
	WorkplaceType      string              `json:"workplaceType"`
	JobURL             string              `json:"jobUrl"`
	DescriptionHtml    string              `json:"descriptionHtml"`
}

type ashbySecondaryLoc struct {
	Location string `json:"location"`
	Address  struct {
		PostalAddress ashbyPostalAddress `json:"postalAddress"`
	} `json:"address"`
}

type ashbyPostalAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressRegion   string `json:"addressRegion"`
	AddressCountry  string `json:"addressCountry"`
}

// ashbyEmploymentAliases maps Ashby employment-type wire values (after the
// shared lowercase + strip-non-alphanum normalization) to schema enum values.
// Source: Ashby's documented enum (`FullTime | PartTime | Contract | Intern |
// Temporary`). Casing variants normalize through stripNonAlphaNumLower so
// e.g. `Full-Time` would also match `fulltime`.
var ashbyEmploymentAliases = map[string]string{
	"fulltime":  "full_time",
	"parttime":  "part_time",
	"contract":  "contract",
	"intern":    "intern",
	"temporary": "temporary",
}

// FetchPostings retrieves all jobs for the given Ashby clientname and
// normalizes them into Postings. Single request — Ashby's public API returns
// the full board in one response. Any network, HTTP, or parse failure aborts
// the whole fetch — no partial successes.
func (a *Ashby) FetchPostings(ctx context.Context, clientName string) ([]domain.Posting, error) {
	escapedName := url.PathEscape(clientName)
	fetchURL := fmt.Sprintf("%s/%s", a.baseURL, escapedName)

	body, err := httpFetch(ctx, a.client, fetchURL)
	if err != nil {
		return nil, fmt.Errorf("ashby: fetching postings for %s: %w", clientName, err)
	}

	var wire ashbyResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("ashby: decoding response for %s: %w", clientName, err)
	}

	postings := make([]domain.Posting, 0, len(wire.Jobs))
	for i, raw := range wire.Jobs {
		posting, err := decodeAshbyJob(raw, clientName, i)
		if err != nil {
			return nil, err
		}
		postings = append(postings, posting)
	}

	return postings, nil
}

func decodeAshbyJob(raw json.RawMessage, clientName string, index int) (domain.Posting, error) {
	var job ashbyJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return domain.Posting{}, fmt.Errorf("ashby: decoding job at index %d for %s: %w", index, clientName, err)
	}

	if job.ID == "" {
		return domain.Posting{}, fmt.Errorf("ashby: job at index %d for %s has missing id", index, clientName)
	}
	if job.JobURL == "" {
		return domain.Posting{}, fmt.Errorf("ashby: job %s for %s has missing jobUrl", job.ID, clientName)
	}

	posting := domain.Posting{
		SourceID:  job.ID,
		SourceURL: job.JobURL,
		JobURL:    ptrIfNonEmpty(job.JobURL),
		RawData:   raw,
	}

	posting.Title = ptrIfNonEmpty(job.Title)
	posting.Team = ptrIfNonEmpty(job.Team)
	posting.Department = ptrIfNonEmpty(job.Department)

	posting.LocationTexts = renderAshbyLocations(job.Location, job.SecondaryLocations)
	if len(posting.LocationTexts) > 0 {
		first := posting.LocationTexts[0]
		posting.LocationText = &first
	}

	posting.WorkplaceType = normalizeAshbyWorkplaceType(job.WorkplaceType)
	posting.EmploymentType = normalizeAshbyEmploymentType(job.EmploymentType)

	if text := htmlToPlainText(job.DescriptionHtml); text != "" {
		posting.DescriptionText = &text
	}

	// Ashby compensation deferred; see fetch-runs-and-richer-snapshots spec.
	// The public job-board API exposes a `compensation` block, but normalizing
	// its tiered/component shape into the schema's flat min/max/currency/period
	// model is out of scope for this iteration. All four comp fields stay nil.

	if job.PublishedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, job.PublishedAt)
		if err != nil {
			return domain.Posting{}, fmt.Errorf("ashby: job %s for %s: parse publishedAt %q: %w", job.ID, clientName, job.PublishedAt, err)
		}
		t = t.UTC()
		posting.PostedAt = &t
		posting.SourceFirstPublishedAt = &t
		// Ashby's public API exposes no `updatedAt` or last-modified field;
		// SourceLastModifiedAt stays nil.
	}

	return posting, nil
}

// renderAshbyLocations builds the LocationTexts array as
// `[location] ++ rendered(secondaryLocations)`. The primary `location` string
// is appended verbatim.
//
// why: each secondary on the wire carries both a free-form `location` string
// and a structured `address.postalAddress`. Either, both, or neither may be
// populated. Resolution rule, in order:
//  1. Prefer the rendered postalAddress (`locality, region, country` with
//     empty parts skipped) when it is non-empty — it is the more structured
//     signal and matches the primary's regional formatting.
//  2. Fall back to the secondary's `location` string when the postal render
//     is empty but `location` is populated. Skipping this fallback silently
//     drops entries like `{"location": "Tokyo Office", "address": {...empty}}`
//     that real boards return.
//  3. Drop the secondary entirely when both are empty so LocationTexts never
//     contains blank entries.
func renderAshbyLocations(primary string, secondary []ashbySecondaryLoc) []string {
	var out []string
	if primary != "" {
		out = append(out, primary)
	}
	for _, s := range secondary {
		if rendered := renderAshbyPostalAddress(s.Address.PostalAddress); rendered != "" {
			out = append(out, rendered)
			continue
		}
		if s.Location != "" {
			out = append(out, s.Location)
		}
	}
	return out
}

// renderAshbyPostalAddress joins locality, region, country with ", ", skipping
// empty parts so sparsely-populated addresses render as e.g. "Germany" rather
// than ", , Germany".
func renderAshbyPostalAddress(p ashbyPostalAddress) string {
	parts := make([]string, 0, 3)
	if p.AddressLocality != "" {
		parts = append(parts, p.AddressLocality)
	}
	if p.AddressRegion != "" {
		parts = append(parts, p.AddressRegion)
	}
	if p.AddressCountry != "" {
		parts = append(parts, p.AddressCountry)
	}
	return strings.Join(parts, ", ")
}

// normalizeAshbyWorkplaceType maps Ashby's PascalCase wire value to the schema
// enum (`onsite | hybrid | remote`). The rule (lowercase + match) handles
// `OnSite`, `Hybrid`, `Remote`, and any future casing variant uniformly. Empty
// or unrecognized values return nil; unrecognized values also emit an [ashby]
// warn naming the wire string so genuinely-new values surface without
// aborting the fetch.
func normalizeAshbyWorkplaceType(raw string) *string {
	if raw == "" {
		return nil
	}
	v := strings.ToLower(raw)
	switch v {
	case "onsite", "hybrid", "remote":
		return &v
	default:
		slog.Warn("[ashby] unknown workplaceType", "value", raw)
		return nil
	}
}

// normalizeAshbyEmploymentType maps Ashby's `employmentType` enum to the schema
// enum via the lowercase + strip-non-alphanum + alias-map rule shared with
// Lever. Resilient to casing variants. Unknown values return nil and emit an
// [ashby] warn naming the wire string.
func normalizeAshbyEmploymentType(raw string) *string {
	if raw == "" {
		return nil
	}
	key := stripNonAlphaNumLower(raw)
	if key == "" {
		return nil
	}
	if v, ok := ashbyEmploymentAliases[key]; ok {
		return &v
	}
	slog.Warn("[ashby] unknown employmentType", "value", raw)
	return nil
}
