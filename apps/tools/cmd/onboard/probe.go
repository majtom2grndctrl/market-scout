package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

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

// detectedATS is what ATS detection produces from a careers URL. Either
// fields are populated and recognized==true, or recognized==false (the URL
// matched no known pattern).
type detectedATS struct {
	recognized bool
	ats        string
	boardToken string
}

// atsPattern captures one URL-to-ATS detection rule. The order of patterns
// in atsDetectionRules below is load-bearing: more specific patterns must
// come before more general ones (e.g. job-boards.greenhouse.io before any
// future wildcard greenhouse rule).
type atsPattern struct {
	ats     string
	regex   *regexp.Regexp
	extract func(m []string) string // takes regex submatches, returns board_token
}

var atsDetectionRules = []atsPattern{
	{
		ats:     "greenhouse",
		regex:   regexp.MustCompile(`(?i)^https?://boards\.greenhouse\.io/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		ats:     "greenhouse",
		regex:   regexp.MustCompile(`(?i)^https?://job-boards\.greenhouse\.io/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		ats:     "lever",
		regex:   regexp.MustCompile(`(?i)^https?://jobs\.lever\.co/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		ats:     "ashby",
		regex:   regexp.MustCompile(`(?i)^https?://jobs\.ashbyhq\.com/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		// Workday: host.myworkdayjobs.com[/locale]/site
		// Locale is optional in URLs we see in research lists; strip it when
		// present per watchlist.md §Workday token discovery. board_token is
		// {host}/{site} — locale dropped.
		ats:   "workday",
		regex: regexp.MustCompile(`(?i)^https?://([a-z0-9.-]+\.myworkdayjobs\.com)(?:/[a-z]{2}(?:-[A-Z]{2})?)?/([^/?#]+)`),
		extract: func(m []string) string {
			return strings.ToLower(m[1]) + "/" + m[2]
		},
	},
	{
		// Workable: lowercased slug.
		ats:     "workable",
		regex:   regexp.MustCompile(`(?i)^https?://apply\.workable\.com/([^/?#]+)`),
		extract: func(m []string) string { return strings.ToLower(m[1]) },
	},
}

// detectATS maps a careers-page URL to an ATS + board token. First-match
// wins, matching the precedence documented in watchlist.md §ATS detection.
func detectATS(careersURL string) detectedATS {
	for _, rule := range atsDetectionRules {
		if m := rule.regex.FindStringSubmatch(careersURL); m != nil {
			return detectedATS{
				recognized: true,
				ats:        rule.ats,
				boardToken: rule.extract(m),
			}
		}
	}
	return detectedATS{recognized: false}
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
	req.Header.Set("User-Agent", "market-scout/0.1 (onboarding probe; +https://github.com/majtom2grndctrl/market-scout/apps/tools)")
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
// (nil, false) if the value is not recognized as a supported ATS. Supported
// values: greenhouse, lever, ashby, workday, workable — the same supported
// set as cmd/fetcher.
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
)
