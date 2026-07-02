// Package atsdetect owns deterministic ATS URL detection and input validation.
// It performs no network or database work.
// See agent-context/lib/watchlist.md for supported ATS detection semantics.
package atsdetect

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	StatusDetected       = "detected"
	StatusAmbiguous      = "ambiguous"
	StatusUnsupportedATS = "unsupported-ats"
	StatusInvalidInput   = "invalid-input"

	CodeInvalidURL = "invalid_url"
)

// Detection describes one ATS match from one supplied URL. Recognized is false
// when the URL matches no supported ATS pattern.
type Detection struct {
	Recognized bool   `json:"recognized"`
	ATS        string `json:"ats"`
	BoardToken string `json:"board_token"`
	SourceURL  string `json:"source_url"`
	SourceKind string `json:"source_kind"`
	Pattern    string `json:"pattern"`
}

// Result is the pure parsing result consumed by callers that need to map URL
// evidence into an ATS and board token.
type Result struct {
	Status   string        `json:"status"`
	Selected *Detection    `json:"selected"`
	Matches  []Detection   `json:"matches"`
	Errors   []ActionError `json:"errors"`
}

// ActionError mirrors the structured validation error shape used by MCP action
// envelopes.
type ActionError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SupportedATS returns the supported ATS keys in stable order.
func SupportedATS() []string {
	return []string{"greenhouse", "lever", "ashby", "workday", "workable"}
}

// ValidateATS rejects ATS keys outside the closed supported set.
func ValidateATS(ats string) error {
	if !isSupportedATS(ats) {
		return fmt.Errorf("unsupported ats %q", ats)
	}
	return nil
}

// ValidateBoardToken applies ATS-specific board-token syntax rules. Greenhouse,
// Lever, and Ashby accept any non-empty token; Workday and Workable are stricter
// at the action boundary.
func ValidateBoardToken(ats, boardToken string) error {
	if boardToken == "" {
		return fmt.Errorf("board_token is required")
	}
	if err := ValidateATS(ats); err != nil {
		return err
	}

	switch ats {
	case "workday":
		return validateWorkdayBoardToken(boardToken)
	case "workable":
		if !workableSlugPattern.MatchString(boardToken) {
			return fmt.Errorf("workable board_token must be a lowercase slug like %q", "acme-co")
		}
	}
	return nil
}

// ValidateURL accepts only absolute http or https URLs.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("url is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must be an absolute http(s) URL")
	}
	if u.Host == "" {
		return fmt.Errorf("url must be an absolute http(s) URL")
	}
	return nil
}

// DetectURL maps a single URL to an ATS and board token. First pattern match
// wins. The function is pure pattern detection and does not validate URL shape.
func DetectURL(rawURL, sourceKind string) Detection {
	for _, rule := range detectionRules {
		if m := rule.regex.FindStringSubmatch(rawURL); m != nil {
			return Detection{
				Recognized: true,
				ATS:        rule.ats,
				BoardToken: rule.extract(m),
				SourceURL:  rawURL,
				SourceKind: sourceKind,
				Pattern:    rule.pattern,
			}
		}
	}
	return Detection{
		Recognized: false,
		SourceURL:  rawURL,
		SourceKind: sourceKind,
	}
}

// DetectEvidence validates supplied URL evidence, detects supported ATS URL
// patterns, and dedupes matches by ATS and board token. It does not validate
// captured board tokens; add_company remains the validation and verification
// gate.
func DetectEvidence(careersURL string, observedURLs []string) (Result, error) {
	evidence := make([]urlEvidence, 0, 1+len(observedURLs))
	if careersURL != "" {
		evidence = append(evidence, urlEvidence{
			path:       "careers_url",
			sourceKind: "careers_url",
			rawURL:     careersURL,
		})
	}
	for i, rawURL := range observedURLs {
		evidence = append(evidence, urlEvidence{
			path:       fmt.Sprintf("observed_urls[%d]", i),
			sourceKind: "observed_url",
			rawURL:     rawURL,
		})
	}

	var errs []ActionError
	for _, item := range evidence {
		if err := ValidateURL(item.rawURL); err != nil {
			errs = append(errs, ActionError{
				Path:    item.path,
				Code:    CodeInvalidURL,
				Message: err.Error(),
			})
		}
	}
	if len(errs) > 0 {
		return Result{Status: StatusInvalidInput, Errors: errs}, nil
	}

	matches := make([]Detection, 0, len(evidence))
	seen := make(map[string]bool)
	for _, item := range evidence {
		det := DetectURL(item.rawURL, item.sourceKind)
		if !det.Recognized {
			continue
		}
		key := det.ATS + "\x00" + det.BoardToken
		if seen[key] {
			continue
		}
		seen[key] = true
		matches = append(matches, det)
	}

	switch len(matches) {
	case 0:
		return Result{Status: StatusUnsupportedATS, Matches: []Detection{}, Errors: []ActionError{}}, nil
	case 1:
		selected := matches[0]
		return Result{Status: StatusDetected, Selected: &selected, Matches: matches, Errors: []ActionError{}}, nil
	default:
		return Result{Status: StatusAmbiguous, Matches: matches, Errors: []ActionError{}}, nil
	}
}

type urlEvidence struct {
	path       string
	sourceKind string
	rawURL     string
}

type detectionRule struct {
	ats     string
	pattern string
	regex   *regexp.Regexp
	extract func(m []string) string
}

// detectionRules is ordered; the first matching pattern wins.
var detectionRules = []detectionRule{
	{
		ats:     "greenhouse",
		pattern: "greenhouse_boards",
		regex:   regexp.MustCompile(`(?i)^https?://boards\.greenhouse\.io/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		ats:     "greenhouse",
		pattern: "greenhouse_job_boards",
		regex:   regexp.MustCompile(`(?i)^https?://job-boards\.greenhouse\.io/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		ats:     "lever",
		pattern: "lever",
		regex:   regexp.MustCompile(`(?i)^https?://jobs\.lever\.co/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		ats:     "ashby",
		pattern: "ashby",
		regex:   regexp.MustCompile(`(?i)^https?://jobs\.ashbyhq\.com/([^/?#]+)`),
		extract: func(m []string) string { return m[1] },
	},
	{
		ats:     "workday",
		pattern: "workday",
		regex:   regexp.MustCompile(`(?i)^https?://([a-z0-9.-]+\.myworkdayjobs\.com)(?:/[a-z]{2}(?:-[A-Z]{2})?)?/([^/?#]+)`),
		extract: func(m []string) string {
			return strings.ToLower(m[1]) + "/" + m[2]
		},
	},
	{
		ats:     "workable",
		pattern: "workable",
		regex:   regexp.MustCompile(`(?i)^https?://apply\.workable\.com/([^/?#]+)`),
		extract: func(m []string) string { return strings.ToLower(m[1]) },
	},
}

var (
	workdayHostPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*\.myworkdayjobs\.com$`)
	workableSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

func isSupportedATS(ats string) bool {
	switch ats {
	case "greenhouse", "lever", "ashby", "workday", "workable":
		return true
	default:
		return false
	}
}

func validateWorkdayBoardToken(boardToken string) error {
	parts := strings.Split(boardToken, "/")
	if len(parts) != 2 {
		return fmt.Errorf("workday board_token must be {host}/{site}")
	}
	host, site := parts[0], parts[1]
	if !workdayHostPattern.MatchString(host) {
		return fmt.Errorf("workday host must match *.myworkdayjobs.com")
	}
	if site == "" {
		return fmt.Errorf("workday site must not be empty")
	}
	if strings.ContainsAny(site, "/?#") {
		return fmt.Errorf("workday site must not contain %q", "/?#")
	}
	return nil
}
