package main

import (
	"regexp"
	"strings"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/seniority"
)

// wordRun splits a title into the words a mask must match. Punctuation and
// spacing between words are matched loosely, so "Engineer, Payments" in the
// title masks "Engineer - Payments" in the body.
var wordRun = regexp.MustCompile(`[\p{L}\p{N}]+`)

// maskTitle removes the posting's title from the role question's state, and
// the title stripped of level words with it. Research measures how far a title
// strays from its role; a role question that can read the title measures the
// title instead (research.md §Title leakage).
func maskTitle(description, title string) string {
	masked := description
	for _, t := range maskTargets(title) {
		if re := titlePattern(t); re != nil {
			masked = re.ReplaceAllString(masked, maskPlaceholder)
		}
	}
	return masked
}

// maskTargets lists the full title first, so a stripped title that is a
// substring of it never splits a full-title match.
func maskTargets(title string) []string {
	targets := []string{title}
	if stripped := seniority.StripLevelWords(title); stripped != "" && !strings.EqualFold(stripped, title) {
		targets = append(targets, stripped)
	}
	return targets
}

// titlePattern matches a title's words in order, case-insensitively, separated
// by any run of non-alphanumerics. It deliberately has no word edges. Stripped
// HTML glues words together ("Founding Software EngineerYou'll be"), and a
// longer form still carries the title ("software engineering"); either would
// leak the title past an edge-bounded match. Masking the title text wherever it
// appears leaves "[—]ing", which no longer names it.
func titlePattern(title string) *regexp.Regexp {
	words := wordRun.FindAllString(title, -1)
	if len(words) == 0 {
		return nil
	}
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`(?i)` + strings.Join(words, `[^\p{L}\p{N}]+`))
}
