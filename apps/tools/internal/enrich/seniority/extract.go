package seniority

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Location says whether a candidate came from the title or the body, which
// decides its v9 tag.
type Location string

const (
	LocationTitle Location = "title"
	LocationBody  Location = "body"
)

// Candidate is one level phrase found in the text, to be judged by the caller.
type Candidate struct {
	// Phrase is the verbatim source text, the whole band or compound when
	// words were merged. It is what the note quotes.
	Phrase string
	// Context is a short verbatim window around the phrase, roughly its
	// sentence, for the caller's question.
	Context  string
	Location Location
	// Offset is the phrase's byte offset in its source string.
	Offset int
	// Value is the Step-1 value if the phrase acts as a level, or "" for an
	// adjacent pair missing from v9's compound table.
	Value string
}

// Extract finds every candidate in title, then in body, each in source order.
func Extract(title, body string) []Candidate {
	return append(extractFrom(title, LocationTitle), extractFrom(body, LocationBody)...)
}

// extractRE matches any entry of levelWords, one capture group per entry so a
// match maps back to its row. It has no leading \b: stripped HTML glues words
// ("RoleSenior"), so the word edge is checked after matching by atWordStart.
// The trailing edge stays in each pattern, which is what keeps "Staffing",
// "Internal" and "Leadership" from matching.
var extractRE = func() *regexp.Regexp {
	groups := make([]string, len(levelWords))
	for i, w := range levelWords {
		groups[i] = "(" + w.pattern + ")"
	}
	return regexp.MustCompile(`(?i)(?:` + strings.Join(groups, `|`) + `)`)
}()

type token struct {
	start, end int
	levelWord
}

func findTokens(s string) []token {
	var out []token
	for _, m := range extractRE.FindAllStringSubmatchIndex(s, -1) {
		if !atWordStart(s, m[0]) {
			continue
		}
		for g := range levelWords {
			if m[2+2*g] >= 0 {
				out = append(out, token{start: m[0], end: m[1], levelWord: levelWords[g]})
				break
			}
		}
	}
	return out
}

// atWordStart accepts a match at a word edge, or one glued to a preceding
// lowercase letter when the match itself is capitalised: "RoleSenior" is two
// words run together, "misleader" is one word.
func atWordStart(s string, i int) bool {
	if i == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(s[:i])
	if !unicode.IsLetter(prev) && !unicode.IsDigit(prev) && prev != '_' {
		return true
	}
	first, _ := utf8.DecodeRuneInString(s[i:])
	return unicode.IsLower(prev) && unicode.IsUpper(first)
}

var (
	adjacentGapRE = regexp.MustCompile(`^\s*$`)
	hyphenGapRE   = regexp.MustCompile(`^\s*[-‐–]\s*$`)
	bandGapRE     = regexp.MustCompile(`(?i)^(?:\s*[/+]\s*|\s+(?:or|to)\s+)$`)
)

type joint int

const (
	jointBreak    joint = iota // separate phrases
	jointCompound              // one rung the employer coined: resolve by table
	jointBand                  // either level: take the lower bound
)

// join decides how two neighbouring tokens relate. A hyphen is ambiguous in
// v9: "senior-staff" is a compound and "Mid-Senior" a band. The table breaks
// the tie, since a band needs no table row and a compound must have one.
// A partner joins only through the table, so "Lead Architect" stays "Lead".
func join(s string, a, b token) joint {
	gap := s[a.end:b.start]
	_, inTable := compounds[[2]string{a.key, b.key}]
	partner := a.partner || b.partner
	switch {
	case adjacentGapRE.MatchString(gap):
		if partner && !inTable {
			return jointBreak
		}
		return jointCompound
	case hyphenGapRE.MatchString(gap):
		if inTable {
			return jointCompound
		}
		if partner {
			return jointBreak
		}
		return jointBand
	case bandGapRE.MatchString(gap) && !partner:
		return jointBand
	}
	return jointBreak
}

// ofEndRE and nextWordRE extend "Head of"/"VP of" by the function it heads,
// so the note quotes "Head of Design" rather than a dangling "Head of".
var (
	ofEndRE    = regexp.MustCompile(`(?i)\sof$`)
	nextWordRE = regexp.MustCompile(`^\s+[\p{L}\p{N}][\p{L}\p{N}&'-]*`)
)

func extractFrom(s string, loc Location) []Candidate {
	tokens := findTokens(s)
	var out []Candidate
	for i := 0; i < len(tokens); {
		segments := [][]token{{tokens[i]}}
		j := i + 1
		for ; j < len(tokens); j++ {
			switch join(s, tokens[j-1], tokens[j]) {
			case jointCompound:
				last := len(segments) - 1
				segments[last] = append(segments[last], tokens[j])
				continue
			case jointBand:
				segments = append(segments, []token{tokens[j]})
				continue
			}
			break
		}
		first, last := tokens[i], tokens[j-1]
		i = j
		if len(segments) == 1 && len(segments[0]) == 1 && first.partner {
			continue
		}
		end := last.end
		if (last.key == "head" || last.key == "vp") && ofEndRE.MatchString(s[last.start:last.end]) {
			if m := nextWordRE.FindStringIndex(s[end:]); m != nil {
				end += m[1]
			}
		}
		out = append(out, Candidate{
			Phrase:   s[first.start:end],
			Context:  contextAround(s, first.start, end),
			Location: loc,
			Offset:   first.start,
			Value:    bandValue(segments),
		})
	}
	return out
}

// bandValue is the lower bound over a band's members, or "" when any member is
// a compound v9's table does not cover: the bound is unknowable then.
func bandValue(segments [][]token) string {
	best := ""
	for _, seg := range segments {
		v := compoundValue(seg)
		if v == "" {
			return ""
		}
		if best == "" || rank[v] < rank[best] {
			best = v
		}
	}
	return best
}

// compoundValue resolves one run of adjacent words. Every partner row in v9's
// table resolves to its level word's own value, so past a two-word run only
// the level words decide; "Senior Principal Architect" is "senior principal".
// A run of three level words has no table row and stays uncovered.
func compoundValue(seg []token) string {
	if len(seg) == 1 {
		return seg[0].value
	}
	if len(seg) == 2 {
		return compounds[[2]string{seg[0].key, seg[1].key}]
	}
	var core []token
	for _, t := range seg {
		if !t.partner {
			core = append(core, t)
		}
	}
	switch len(core) {
	case 1:
		return core[0].value
	case 2:
		return compounds[[2]string{core[0].key, core[1].key}]
	}
	return ""
}

// contextCap bounds the window handed to the judge; a sentence longer than
// this is cut around the phrase.
const contextCap = 300

// contextAround returns the sentence holding s[start:end], cut to contextCap
// at word edges. Body text arrives whitespace-collapsed, so ". " is the only
// sentence edge left; an abbreviation ends a sentence early, which only
// shortens the window.
func contextAround(s string, start, end int) string {
	lo := 0
	if k := lastSentenceEnd(s[:start]); k >= 0 {
		lo = k
	}
	hi := len(s)
	if k := firstSentenceEnd(s[end:]); k >= 0 {
		hi = end + k
	}
	if hi-lo > contextCap {
		room := max(0, (contextCap-(end-start))/2)
		if start-room > lo {
			lo = snapForward(s, start-room, start)
		}
		if end+room < hi {
			hi = snapBack(s, end+room, end)
		}
	}
	return strings.TrimSpace(s[lo:hi])
}

// lastSentenceEnd is the index just past the last ". ", "! " or "? " in s.
func lastSentenceEnd(s string) int {
	best := -1
	for _, p := range []string{". ", "! ", "? "} {
		if k := strings.LastIndex(s, p); k >= 0 && k+len(p) > best {
			best = k + len(p)
		}
	}
	return best
}

// firstSentenceEnd is the index just past the first ".", "!" or "?" in s that
// is followed by a space or ends s.
func firstSentenceEnd(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', '!', '?':
			if i+1 == len(s) || s[i+1] == ' ' {
				return i + 1
			}
		}
	}
	return -1
}

// snapForward moves i forward to the next space, never past limit, so the
// window starts on a word and on a rune boundary.
func snapForward(s string, i, limit int) int {
	if k := strings.IndexByte(s[i:limit], ' '); k >= 0 {
		return i + k + 1
	}
	for i < limit && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}

// snapBack moves i back to the previous space, never before limit.
func snapBack(s string, i, limit int) int {
	if k := strings.LastIndexByte(s[limit:i], ' '); k >= 0 {
		return limit + k
	}
	for i > limit && i < len(s) && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}
