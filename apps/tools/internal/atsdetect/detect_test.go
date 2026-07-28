package atsdetect

import (
	"reflect"
	"strings"
	"testing"
)

func TestDetectURL_SupportedPatterns(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantATS string
		wantTok string
		wantPat string
	}{
		{
			name:    "greenhouse boards",
			rawURL:  "https://boards.greenhouse.io/acme",
			wantATS: "greenhouse",
			wantTok: "acme",
			wantPat: "greenhouse_boards",
		},
		{
			name:    "greenhouse job boards",
			rawURL:  "https://job-boards.greenhouse.io/acme/jobs",
			wantATS: "greenhouse",
			wantTok: "acme",
			wantPat: "greenhouse_job_boards",
		},
		{
			// Regression: mixed-case greenhouse tokens must normalize to
			// lowercase so "Stripe" and "stripe" resolve to the same company.
			name:    "greenhouse boards lowercases mixed-case token",
			rawURL:  "https://boards.greenhouse.io/Stripe",
			wantATS: "greenhouse",
			wantTok: "stripe",
			wantPat: "greenhouse_boards",
		},
		{
			name:    "lever",
			rawURL:  "https://jobs.lever.co/acme",
			wantATS: "lever",
			wantTok: "acme",
			wantPat: "lever",
		},
		{
			// Lever's API is case-sensitive (see atsdetect.NormalizeBoardToken);
			// detection must preserve the captured casing exactly.
			name:    "lever preserves mixed-case token",
			rawURL:  "https://jobs.lever.co/MastReforestation",
			wantATS: "lever",
			wantTok: "MastReforestation",
			wantPat: "lever",
		},
		{
			name:    "ashby",
			rawURL:  "https://jobs.ashbyhq.com/acme",
			wantATS: "ashby",
			wantTok: "acme",
			wantPat: "ashby",
		},
		{
			// Regression: mixed-case ashby tokens must normalize to lowercase
			// so "QAWolf" and "qawolf" resolve to the same company.
			name:    "ashby lowercases mixed-case token",
			rawURL:  "https://jobs.ashbyhq.com/QAWolf",
			wantATS: "ashby",
			wantTok: "qawolf",
			wantPat: "ashby",
		},
		{
			name:    "workday strips locale",
			rawURL:  "https://acme.wd5.myworkdayjobs.com/en-US/AcmeCareers/jobs",
			wantATS: "workday",
			wantTok: "acme.wd5.myworkdayjobs.com/AcmeCareers",
			wantPat: "workday",
		},
		{
			// Workday host is DNS (case-insensitive) and lowercases; site is an
			// opaque tenant path segment and is preserved as captured.
			name:    "workday lowercases host but preserves site casing",
			rawURL:  "https://Acme.WD5.MyWorkdayJobs.com/en-US/AcmeCareers/jobs",
			wantATS: "workday",
			wantTok: "acme.wd5.myworkdayjobs.com/AcmeCareers",
			wantPat: "workday",
		},
		{
			name:    "workday no locale",
			rawURL:  "https://acme.wd5.myworkdayjobs.com/AcmeCareers",
			wantATS: "workday",
			wantTok: "acme.wd5.myworkdayjobs.com/AcmeCareers",
			wantPat: "workday",
		},
		{
			name:    "workable lowercases token",
			rawURL:  "https://apply.workable.com/AcmeRobotics/",
			wantATS: "workable",
			wantTok: "acmerobotics",
			wantPat: "workable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectURL(tc.rawURL, "careers_url")
			if !got.Recognized {
				t.Fatalf("Recognized = false, want true")
			}
			if got.ATS != tc.wantATS {
				t.Errorf("ATS = %q, want %q", got.ATS, tc.wantATS)
			}
			if got.BoardToken != tc.wantTok {
				t.Errorf("BoardToken = %q, want %q", got.BoardToken, tc.wantTok)
			}
			if got.SourceURL != tc.rawURL {
				t.Errorf("SourceURL = %q, want %q", got.SourceURL, tc.rawURL)
			}
			if got.SourceKind != "careers_url" {
				t.Errorf("SourceKind = %q, want careers_url", got.SourceKind)
			}
			if got.Pattern != tc.wantPat {
				t.Errorf("Pattern = %q, want %q", got.Pattern, tc.wantPat)
			}
		})
	}
}

func TestDetectURL_UnsupportedPattern(t *testing.T) {
	got := DetectURL("https://careers.random.example.com/jobs", "careers_url")
	if got.Recognized {
		t.Fatalf("Recognized = true, want false")
	}
	if got.ATS != "" || got.BoardToken != "" || got.Pattern != "" {
		t.Fatalf("got populated detection fields: %+v", got)
	}
}

func TestDetectEvidence_SelectedDedupesMatches(t *testing.T) {
	got, err := DetectEvidence("https://boards.greenhouse.io/acme", []string{
		"https://boards.greenhouse.io/acme/jobs",
	})
	if err != nil {
		t.Fatalf("DetectEvidence error: %v", err)
	}
	if got.Status != StatusDetected {
		t.Fatalf("Status = %q, want %q", got.Status, StatusDetected)
	}
	if got.Selected == nil {
		t.Fatalf("Selected = nil, want detection")
	}
	if got.Selected.ATS != "greenhouse" || got.Selected.BoardToken != "acme" {
		t.Fatalf("Selected = %+v, want greenhouse/acme", *got.Selected)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("len(Matches) = %d, want 1", len(got.Matches))
	}
}

func TestDetectEvidence_DedupesCasingVariantsOfSameToken(t *testing.T) {
	// Regression for the gap that let "QAWolf" and "qawolf" enter as distinct
	// companies: the ashby rule returned the raw captured token with no
	// normalization, so casing variants of the same board dedupe-keyed
	// differently. Normalization now collapses them to one match.
	got, err := DetectEvidence("https://jobs.ashbyhq.com/QAWolf", []string{
		"https://jobs.ashbyhq.com/qawolf",
	})
	if err != nil {
		t.Fatalf("DetectEvidence error: %v", err)
	}
	if got.Status != StatusDetected {
		t.Fatalf("Status = %q, want %q", got.Status, StatusDetected)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("len(Matches) = %d, want 1 (casing variants should dedupe): %+v", len(got.Matches), got.Matches)
	}
	if got.Selected == nil || got.Selected.BoardToken != "qawolf" {
		t.Fatalf("Selected = %+v, want ashby/qawolf", got.Selected)
	}
}

func TestDetectEvidence_AmbiguousDistinctMatches(t *testing.T) {
	got, err := DetectEvidence("https://boards.greenhouse.io/acme", []string{
		"https://jobs.lever.co/acme",
	})
	if err != nil {
		t.Fatalf("DetectEvidence error: %v", err)
	}
	if got.Status != StatusAmbiguous {
		t.Fatalf("Status = %q, want %q", got.Status, StatusAmbiguous)
	}
	if got.Selected != nil {
		t.Fatalf("Selected = %+v, want nil", *got.Selected)
	}
	if len(got.Matches) != 2 {
		t.Fatalf("len(Matches) = %d, want 2", len(got.Matches))
	}
}

func TestDetectEvidence_UnsupportedATS(t *testing.T) {
	got, err := DetectEvidence("https://careers.example.com/jobs", nil)
	if err != nil {
		t.Fatalf("DetectEvidence error: %v", err)
	}
	if got.Status != StatusUnsupportedATS {
		t.Fatalf("Status = %q, want %q", got.Status, StatusUnsupportedATS)
	}
	if got.Selected != nil {
		t.Fatalf("Selected = %+v, want nil", *got.Selected)
	}
	if len(got.Matches) != 0 {
		t.Fatalf("len(Matches) = %d, want 0", len(got.Matches))
	}
}

func TestDetectEvidence_InvalidInput(t *testing.T) {
	got, err := DetectEvidence("not a url", []string{"ftp://example.com/jobs", ""})
	if err != nil {
		t.Fatalf("DetectEvidence error: %v", err)
	}
	if got.Status != StatusInvalidInput {
		t.Fatalf("Status = %q, want %q", got.Status, StatusInvalidInput)
	}
	if got.Selected != nil {
		t.Fatalf("Selected = %+v, want nil", *got.Selected)
	}
	if !hasActionError(got.Errors, "careers_url", CodeInvalidURL) {
		t.Fatalf("Errors = %+v, want careers_url invalid_url", got.Errors)
	}
	if !hasActionError(got.Errors, "observed_urls[0]", CodeInvalidURL) {
		t.Fatalf("Errors = %+v, want observed_urls[0] invalid_url", got.Errors)
	}
	if !hasActionError(got.Errors, "observed_urls[1]", CodeInvalidURL) {
		t.Fatalf("Errors = %+v, want observed_urls[1] invalid_url", got.Errors)
	}
}

func TestDetectEvidence_DoesNotValidateCapturedBoardToken(t *testing.T) {
	got, err := DetectEvidence("https://apply.workable.com/Acme_Co", nil)
	if err != nil {
		t.Fatalf("DetectEvidence error: %v", err)
	}
	if got.Status != StatusDetected {
		t.Fatalf("Status = %q, want %q", got.Status, StatusDetected)
	}
	if got.Selected == nil {
		t.Fatalf("Selected = nil, want detection")
	}
	if got.Selected.BoardToken != "acme_co" {
		t.Fatalf("BoardToken = %q, want acme_co", got.Selected.BoardToken)
	}
	if err := ValidateBoardToken(got.Selected.ATS, got.Selected.BoardToken); err == nil {
		t.Fatalf("ValidateBoardToken(%q, %q) = nil, want error", got.Selected.ATS, got.Selected.BoardToken)
	}
}

func TestSupportedATS_StableOrder(t *testing.T) {
	want := []string{"greenhouse", "lever", "ashby", "workday", "workable"}
	if got := SupportedATS(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedATS() = %v, want %v", got, want)
	}
}

func TestValidateATS(t *testing.T) {
	for _, ats := range SupportedATS() {
		if err := ValidateATS(ats); err != nil {
			t.Errorf("ValidateATS(%q) = %v, want nil", ats, err)
		}
	}
	if err := ValidateATS("rippling"); err == nil || !strings.Contains(err.Error(), `unsupported ats "rippling"`) {
		t.Fatalf("ValidateATS(rippling) = %v, want unsupported error", err)
	}
}

func TestValidateBoardToken_Workday(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr string
	}{
		{name: "valid", token: "acme.wd5.myworkdayjobs.com/Careers"},
		{name: "malformed host", token: "acme.example.com/Careers", wantErr: "workday host must match *.myworkdayjobs.com"},
		{name: "missing site", token: "acme.wd5.myworkdayjobs.com", wantErr: "workday board_token must be {host}/{site}"},
		{name: "leading dot", token: ".myworkdayjobs.com/Careers", wantErr: "workday host must match *.myworkdayjobs.com"},
		{name: "empty site", token: "acme.wd5.myworkdayjobs.com/", wantErr: "workday site must not be empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBoardToken("workday", tc.token)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateBoardToken = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("ValidateBoardToken = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateBoardToken_Workable(t *testing.T) {
	if err := ValidateBoardToken("workable", "acme-co"); err != nil {
		t.Fatalf("ValidateBoardToken(valid workable) = %v, want nil", err)
	}
	if err := ValidateBoardToken("workable", "Acme_Co"); err == nil || err.Error() != `workable board_token must be a lowercase slug like "acme-co"` {
		t.Fatalf("ValidateBoardToken(invalid workable) = %v, want slug error", err)
	}
}

func TestValidateBoardToken_GenericATSRequiresNonEmptyToken(t *testing.T) {
	if err := ValidateBoardToken("greenhouse", "acme"); err != nil {
		t.Fatalf("ValidateBoardToken(greenhouse/acme) = %v, want nil", err)
	}
	if err := ValidateBoardToken("greenhouse", ""); err == nil || err.Error() != "board_token is required" {
		t.Fatalf("ValidateBoardToken(greenhouse/empty) = %v, want required error", err)
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr string
	}{
		{name: "https", rawURL: "https://example.com/careers"},
		{name: "http", rawURL: "http://example.com/careers"},
		{name: "bare host", rawURL: "example.com/careers", wantErr: "url must be an absolute http(s) URL"},
		{name: "ftp", rawURL: "ftp://example.com/careers", wantErr: "url must be an absolute http(s) URL"},
		{name: "missing host", rawURL: "https:///careers", wantErr: "url must be an absolute http(s) URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateURL(tc.rawURL)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateURL = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("ValidateURL = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func hasActionError(errs []ActionError, path, code string) bool {
	for _, err := range errs {
		if err.Path == path && err.Code == code {
			return true
		}
	}
	return false
}
