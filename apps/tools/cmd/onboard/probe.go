package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/ats"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/domain"
)

// adapterProbe is the minimal surface cmd/onboard needs from an ATS adapter.
// Decision: reuse FetchPostings as the probe rather than introduce a
// dedicated Probe() method on every adapter. Rationale: a successful
// FetchPostings (any postings count, including zero) is exactly the success
// signal documented in agent-context/lib/watchlist.md §Board token
// verification, and the cost of one full fetch per candidate is bounded —
// 200 records, sequential; all adapters except Workday are single-shot,
// Workday paginates.
// Refactor to a smaller probe only when a research list large enough to
// matter shows up.
type adapterProbe interface {
	FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error)
}

// probeCareersURL issues a GET against the careers URL. Returns nil on 2xx,
// non-nil otherwise. HEAD is deliberately avoided: spec §Per-record behavior
// requires a GET response (some sites disagree between HEAD and GET status
// codes), and the cost of one full GET per record is bounded by the 200-row
// run size.
func probeCareersURL(ctx context.Context, client *http.Client, rawURL string) error {
	if _, err := url.Parse(rawURL); err != nil {
		return fmt.Errorf("invalid careers_url %q: %w", rawURL, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("building careers_url request %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", "market-scout/0.1 (onboarding probe; +https://github.com/majtom2grndctrl/market-scout)")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("careers_url GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("careers_url GET %s: status %d", rawURL, resp.StatusCode)
	}
	return nil
}

// adapterFor returns the live ATS adapter for the given ats string, or
// (nil, false) if the value is not recognized by the fetcher's registered
// adapter set.
func adapterFor(ats string, client *http.Client) (adapterProbe, bool) {
	switch ats {
	case "greenhouse":
		return atsNewGreenhouse(client), true
	case "lever":
		return atsNewLever(client), true
	case "ashby":
		return atsNewAshby(client), true
	case "workday":
		return atsNewWorkday(client), true
	case "workable":
		return atsNewWorkable(client), true
	case "gem":
		return atsNewGem(client), true
	default:
		return nil, false
	}
}

// Constructor indirection: kept here so tests can override the adapter
// factories by swapping the package-level vars (e.g. to point them at an
// httptest.Server). Production uses the real ats.NewXxx constructors.
var (
	atsNewGreenhouse = func(c *http.Client) adapterProbe { return ats.NewGreenhouse(c) }
	atsNewLever      = func(c *http.Client) adapterProbe { return ats.NewLever(c) }
	atsNewAshby      = func(c *http.Client) adapterProbe { return ats.NewAshby(c) }
	atsNewWorkday    = func(c *http.Client) adapterProbe { return ats.NewWorkday(c) }
	atsNewWorkable   = func(c *http.Client) adapterProbe { return ats.NewWorkable(c) }
	atsNewGem        = func(c *http.Client) adapterProbe { return ats.NewGem(c) }
)
