package skillseed

import (
	"regexp"
	"strings"
)

// contextRule disambiguates a short skill name that is also an ordinary word,
// a letter, or a grade. The skill matches only when a Match pattern matches and
// no Exclude pattern does. Patterns are case-sensitive unless they say
// otherwise: "Go" the language is capitalized, "go" the verb usually is not,
// and the case is part of the evidence.
//
// This is the only curated seed data. A rule applies only on an install whose
// taxonomy has a skill named one of Names.
type contextRule struct {
	Key     string
	Names   []string
	Match   []*regexp.Regexp
	Exclude []*regexp.Regexp
}

// langs is the neighbour list that makes a short name read as a programming
// language: "Python, Go", "C and C++".
const langs = `(?:Python|Java|JavaScript|TypeScript|Rust|Kotlin|Scala|Ruby|Swift|Elixir|Erlang|Haskell|PHP|Perl|Node(?:\.js)?|Go|Golang|C\+\+|C#|Objective-C|Assembly|Fortran|MATLAB|SQL|SAS|Stata|Julia|Bash|Shell)`

// sep joins neighbours in a list.
const sep = `\s*(?:,|/|;|\band\b|\bor\b|&)\s*`

// cEnd ends a bare "C" so it cannot be the start of C++ or C#.
const cEnd = `(?:[^+#\w]|$)`

var contextRules = []contextRule{
	{
		Key:   "go",
		Names: []string{"go", "golang", "go (golang)", "go lang"},
		Match: res(
			`(?i)\bgolang\b`,
			`\bGo\b`+sep+langs,
			langs+sep+`\bGo\b`,
			`\bGo\s+(?:programming|language|services|microservices|backend|code(?:base)?|developer|engineer)`,
			`\b(?:in|using|with|written in)\s+Go\b(?:[^-\w]|$)`,
		),
	},
	{
		Key:   "r",
		Names: []string{"r", "r programming", "r (programming language)"},
		Match: res(
			`\bR\b`+sep+langs,
			langs+sep+`\bR\b(?:[^&\w]|$)`,
			`\bR(?:Studio|\s+programming|\s+Shiny)\b`,
			`\b(?:tidyverse|ggplot2?|dplyr|CRAN)\b`,
		),
		Exclude: res(`\bR\s*&\s*D\b`),
	},
	{
		Key:   "c",
		Names: []string{"c", "c programming", "c (programming language)"},
		Match: res(
			`\bC`+sep+langs,
			langs+sep+`\bC`+cEnd,
			`\bC\s*/\s*C\+\+`,
			`\b(?i:embedded|ansi)\s+C`+cEnd,
			`\bC\s+(?:programming|language)\b`,
		),
	},
	{
		Key:   "cpp",
		Names: []string{"c++", "cpp"},
		Match: res(`(?:^|[^\w+])C\+\+`, `(?i)\bcpp\b`),
	},
	{
		Key:     "csharp",
		Names:   []string{"c#", "c sharp", "csharp"},
		Match:   res(`(?:^|[^\w#])C#(?:[^\w#]|$)`, `(?i)\bc-?sharp\b`),
		Exclude: res(`C#\s+(?:major|minor)\b`),
	},
}

var ruleByKey = func() map[string]contextRule {
	m := make(map[string]contextRule, len(contextRules))
	for _, r := range contextRules {
		m[r.Key] = r
	}
	return m
}()

// ruleForName returns the rule covering a skill name, if any.
func ruleForName(name string) (contextRule, bool) {
	key := strings.ToLower(strings.Join(strings.Fields(name), " "))
	for _, r := range contextRules {
		for _, n := range r.Names {
			if n == key {
				return r, true
			}
		}
	}
	return contextRule{}, false
}

func (r contextRule) matches(text string) bool {
	for _, ex := range r.Exclude {
		if ex.MatchString(text) {
			return false
		}
	}
	for _, m := range r.Match {
		if m.MatchString(text) {
			return true
		}
	}
	return false
}

func res(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(p)
	}
	return out
}
