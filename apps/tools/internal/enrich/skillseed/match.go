package skillseed

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

// tokenRun splits text into the units aliases are matched on. A trailing "++"
// or "#" stays on its token, so "C++" and "C#" are tokens of their own and a
// plain "c" never matches inside them.
var tokenRun = regexp.MustCompile(`[\p{L}\p{N}]+(?:\+\+|#)?`)

// tokens lowercases text into match units. Punctuation between words is
// dropped, so "Node.js", "node js" and "Node-JS" tokenize alike.
func tokens(s string) []string {
	return tokenRun.FindAllString(strings.ToLower(s), -1)
}

// maxAliasTokens bounds the phrase index's n-gram length.
const maxAliasTokens = 6

// Matcher finds a seed's lexical skills in text.
type Matcher struct {
	phrases map[string][]int // joined alias tokens → entry indexes
	rules   []ruledEntry
	entries []Entry
}

type ruledEntry struct {
	idx  int
	rule contextRule
}

// NewMatcher indexes the seed's lexical entries.
func NewMatcher(s Seed) *Matcher {
	m := &Matcher{phrases: map[string][]int{}, entries: s.Entries}
	for i, e := range s.Entries {
		if e.Reach != ReachLexical {
			continue
		}
		if e.ContextRule != "" {
			m.rules = append(m.rules, ruledEntry{idx: i, rule: ruleByKey[e.ContextRule]})
			continue
		}
		for _, a := range e.Aliases {
			key := strings.Join(tokens(a), " ")
			if key != "" && !slices.Contains(m.phrases[key], i) {
				m.phrases[key] = append(m.phrases[key], i)
			}
		}
	}
	return m
}

// Match returns the lexical skills whose alias or context rule appears in
// text, in slug order.
func (m *Matcher) Match(text string) []Entry {
	hit := map[int]bool{}
	toks := tokens(text)
	for i := range toks {
		for n := 1; n <= maxAliasTokens && i+n <= len(toks); n++ {
			for _, idx := range m.phrases[strings.Join(toks[i:i+n], " ")] {
				hit[idx] = true
			}
		}
	}
	for _, r := range m.rules {
		if r.rule.matches(text) {
			hit[r.idx] = true
		}
	}
	out := make([]Entry, 0, len(hit))
	for idx := range hit {
		out = append(out, m.entries[idx])
	}
	slices.SortFunc(out, func(a, b Entry) int { return cmp.Compare(a.SkillSlug, b.SkillSlug) })
	return out
}
