// Package seniority is cmd/classify's own seniority instrument: code finds
// candidate level phrases, a decision model judges whether each acts as a
// level, and v9's Step 1 resolves the value.
// See: agent-context/plans/in-progress/hybrid-classifier/index.md
package seniority

import (
	"regexp"
	"strings"
)

// levelWordPatterns is the tool's level-word list. It starts from v9's Step 1
// list (.claude/skills/batch-enrich/SKILL.md) and may drift from it; a change
// there prompts a classify-v review, not an automatic copy. Each entry matches
// the word in any casing, spacing or hyphenation, and the "Sr."/"Jr."
// abbreviations v9 treats as the same form. Manager is deliberately absent: v9
// holds it role-forming, not level-forming.
var levelWordPatterns = []string{
	`intern(?:ship)?\b`,
	`co-?op\b`,
	`apprentice(?:ship)?\b`,
	`new[\s-]*grad(?:uate)?\b`,
	`graduate\b`,
	`entry[\s-]*level\b`,
	`early[\s-]*career\b`,
	`junior\b`, `jr\b\.?`,
	`mid[\s-]*level\b`, `mid\b`,
	`senior\b`, `sr\b\.?`,
	`staff\b`,
	`principal\b`,
	`leader\b`, `lead\b`,
	`director\b`,
	// "Head of" and "VP of" take their connector with them, so stripping a
	// title leaves the function ("Design"), not a dangling "of Design".
	`head(?:\s+of)?\b`,
	`vp(?:\s+of)?\b`, `vice[\s-]+president(?:\s+of)?\b`,
}

// levelWordRE matches one level word starting at a word edge. RE2 has no
// lookaround, so each pattern closes its own edge: \b after a final letter, or
// an optional "." after an abbreviation's edge.
var levelWordRE = regexp.MustCompile(`(?i)\b(?:` + strings.Join(levelWordPatterns, `|`) + `)`)

// StripLevelWords removes every level word from a title, then drops the
// separators the removal orphaned: "Senior Staff Software Engineer" becomes
// "Software Engineer", "Senior/Staff Engineer" becomes "Engineer", and "Head of
// Design" becomes "Design". Hyphens inside a remaining word are kept.
func StripLevelWords(title string) string {
	fields := strings.Fields(levelWordRE.ReplaceAllString(title, " "))
	kept := fields[:0]
	for _, f := range fields {
		if strings.Trim(f, punctuation) != "" {
			kept = append(kept, f)
		}
	}
	return strings.Trim(strings.Join(kept, " "), punctuation+" ")
}

// punctuation is what may be left dangling once a level word is gone.
const punctuation = ",/+&|-–—.:;()"
