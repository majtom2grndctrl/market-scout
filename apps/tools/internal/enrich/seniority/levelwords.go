// Package seniority is cmd/classify's own seniority instrument: code finds
// candidate level phrases, a decision model judges whether each acts as a
// level, and v9's Step 1 resolves the value.
// See: agent-context/plans/in-progress/hybrid-classifier/index.md
package seniority

import (
	"regexp"
	"strings"
)

// The nine values classifications.seniority accepts.
const (
	Intern    = "intern"
	Junior    = "junior"
	Mid       = "mid"
	Senior    = "senior"
	Staff     = "staff"
	Principal = "principal"
	Lead      = "lead"
	Director  = "director"
	Unknown   = "unknown"
)

// levelWord is one entry of the tool's level-word list. key names the word for
// the compound table, so spelling variants ("Sr.", "senior") share one row.
// A partner is a word that is not a level on its own but appears in a v9
// compound ("Senior Manager", "Associate Director"); it only ever joins a
// phrase through that table.
type levelWord struct {
	pattern string
	key     string
	value   string
	partner bool
}

// levelWords is the tool's level-word list. It starts from v9's Step 1 list
// (.claude/skills/batch-enrich/SKILL.md) and may drift from it; a change there
// prompts a classify-v review, not an automatic copy. Each entry matches the
// word in any casing, spacing or hyphenation, and the "Sr."/"Jr."
// abbreviations v9 treats as the same form. Manager is a partner, never a
// level: v9 holds it role-forming, not level-forming.
//
// v9 names the value for Entry-level and Early-career only; the rest of the
// intern and junior rows are this tool's reading of the list. Head and VP
// resolve to director because v9's table sends "Head of X" and "VP of X" there.
var levelWords = []levelWord{
	{pattern: `intern(?:ship)?\b`, key: "intern", value: Intern},
	{pattern: `co-?op\b`, key: "coop", value: Intern},
	{pattern: `apprentice(?:ship)?\b`, key: "apprentice", value: Intern},
	{pattern: `new[\s-]*grad(?:uate)?\b`, key: "newgrad", value: Junior},
	{pattern: `graduate\b`, key: "graduate", value: Junior},
	{pattern: `entry[\s-]*level\b`, key: "entrylevel", value: Junior},
	{pattern: `early[\s-]*career\b`, key: "earlycareer", value: Junior},
	{pattern: `junior\b`, key: "junior", value: Junior},
	{pattern: `jr\b\.?`, key: "junior", value: Junior},
	{pattern: `mid[\s-]*level\b`, key: "mid", value: Mid},
	{pattern: `mid\b`, key: "mid", value: Mid},
	{pattern: `senior\b`, key: "senior", value: Senior},
	{pattern: `sr\b\.?`, key: "senior", value: Senior},
	{pattern: `staff\b`, key: "staff", value: Staff},
	{pattern: `principal\b`, key: "principal", value: Principal},
	{pattern: `leader\b`, key: "lead", value: Lead},
	{pattern: `lead\b`, key: "lead", value: Lead},
	{pattern: `director\b`, key: "director", value: Director},
	// "Head of" and "VP of" take their connector with them, so stripping a
	// title leaves the function ("Design"), not a dangling "of Design".
	{pattern: `head(?:\s+of)?\b`, key: "head", value: Director},
	{pattern: `vp(?:\s+of)?\b`, key: "vp", value: Director},
	{pattern: `vice[\s-]+president(?:\s+of)?\b`, key: "vp", value: Director},

	{pattern: `manager\b`, key: "manager", partner: true},
	{pattern: `architect\b`, key: "architect", partner: true},
	{pattern: `associate\b`, key: "associate", partner: true},
	{pattern: `deputy\b`, key: "deputy", partner: true},
	{pattern: `general\b`, key: "general", partner: true},
}

// compounds is v9's compound table, keyed by the two words in title order.
// "Head of <function>" and "VP of <function>" are single words here, so they
// need no row.
var compounds = map[[2]string]string{
	{"senior", "staff"}:        Staff,
	{"senior", "principal"}:    Principal,
	{"senior", "director"}:     Director,
	{"senior", "manager"}:      Senior,
	{"staff", "principal"}:     Principal,
	{"lead", "staff"}:          Staff,
	{"principal", "architect"}: Principal,
	{"senior", "lead"}:         Lead,
	{"associate", "director"}:  Director,
	{"deputy", "director"}:     Director,
	{"general", "manager"}:     Director,
}

// rank orders values for a band's lower bound. v9 gives no order for lead;
// its table resolves "senior lead" to lead and "lead staff" to staff, which
// places lead between senior and staff.
var rank = map[string]int{
	Intern: 0, Junior: 1, Mid: 2, Senior: 3, Lead: 4, Staff: 5, Principal: 6, Director: 7,
}

// levelWordRE matches one level word starting at a word edge. RE2 has no
// lookaround, so each pattern closes its own edge: \b after a final letter, or
// an optional "." after an abbreviation's edge. Partners are not level words
// and are never stripped.
var levelWordRE = regexp.MustCompile(`(?i)\b(?:` + strings.Join(levelPatterns(), `|`) + `)`)

func levelPatterns() []string {
	var out []string
	for _, w := range levelWords {
		if !w.partner {
			out = append(out, w.pattern)
		}
	}
	return out
}

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
