package ats

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// htmlToPlainText converts an HTML fragment into a flattened plain-text string.
// The pipeline loops `decode entities → drop <script>/<style> elements → strip
// remaining tags` until the working string stabilizes (bounded by maxHTMLPasses
// to cap CPU on adversarial input), then runs a final entity decode for
// surviving body-text entities (e.g. `&amp;` → `&`) and collapses internal
// whitespace runs to single spaces with leading/trailing whitespace trimmed.
//
// Why a loop: Greenhouse's `content` field is *entity-encoded HTML* (e.g.
// `&lt;p&gt;…&lt;/p&gt;`), so a single decode-then-strip pass leaves wrapped
// markup intact. Worse, multiply-encoded payloads (e.g. `&amp;lt;script&amp;gt;…`)
// would survive a fixed two-pass pipeline and re-emerge as literal `<script>`
// in output. Looping until stable catches arbitrary encoding depth while a
// small iteration cap keeps adversarial input from burning unbounded CPU; if
// the cap is hit the working string is returned as-is rather than erroring.
// The function is idempotent on its own output: f(f(x)) == f(x).
//
// Lever (plain HTML) and Ashby (plain HTML) pass through under the same
// pipeline — the loop terminates after one iteration when there is nothing
// left to decode or strip.
//
// Why stdlib-only: ATS adapters are HTTP-only and we keep their dependency
// surface minimal. The intent is "good enough to render Greenhouse/Lever/Ashby
// description content for snapshotting and full-text search," not bug-compatible
// HTML parsing. Malformed input (unclosed tags, unknown entities) is best-effort:
// unknown entities are passed through verbatim.
func htmlToPlainText(s string) string {
	if s == "" {
		return ""
	}

	// Loop decode → drop scripts/styles → strip tags until the string is stable
	// or the iteration cap is reached. This catches multiply-encoded markup
	// (e.g. `&amp;lt;script&amp;gt;`) that a fixed-depth pipeline would miss.
	for i := 0; i < maxHTMLPasses; i++ {
		prev := s
		s = decodeEntities(s)
		s = dropScriptStyle(s)
		s = stripTags(s)
		if s == prev {
			break
		}
	}

	// Final decode pass: handle entities in body text that surfaced after the
	// last strip (e.g. `&amp;` → `&`). The loop above stops as soon as one
	// iteration is a no-op, which can leave one decode unconsumed.
	s = decodeEntities(s)

	// Collapse whitespace runs to single spaces; trim ends.
	var out strings.Builder
	out.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if isASCIISpace(r) {
			inSpace = true
			continue
		}
		if inSpace && out.Len() > 0 {
			out.WriteByte(' ')
		}
		inSpace = false
		out.WriteRune(r)
	}
	return out.String()
}

// maxHTMLPasses caps the decode/strip loop. Five passes is enough to unwrap
// realistic multiply-encoded ATS payloads (one or two layers in practice) with
// margin to spare, while keeping pathological adversarial input bounded.
const maxHTMLPasses = 5

// dropScriptStyle removes entire <script>…</script> and <style>…</style>
// elements, including their bodies. Case-insensitive on the tag name.
func dropScriptStyle(s string) string {
	for _, tag := range []string{"script", "style"} {
		open := "<" + tag
		close := "</" + tag + ">"
		lower := strings.ToLower(s)
		for {
			start := strings.Index(lower, open)
			if start < 0 {
				break
			}
			// Verify the character after the tag name is '>' or whitespace (not a
			// different tag that starts with the same prefix, e.g. <scriptx>).
			afterTag := start + len(open)
			if afterTag < len(s) && s[afterTag] != '>' && s[afterTag] != ' ' &&
				s[afterTag] != '\t' && s[afterTag] != '\n' && s[afterTag] != '\r' &&
				s[afterTag] != '/' {
				// Not a match; advance past this position to avoid an infinite loop.
				// Replace with a non-matching character in the working copy.
				lower = lower[:start] + " " + lower[start+1:]
				continue
			}
			end := strings.Index(lower[start:], close)
			if end < 0 {
				// No closing tag: no complete block to strip, so stop processing
				// this tag type. Breaking avoids silent truncation of the tail.
				break
			} else {
				remove := end + len(close)
				s = s[:start] + s[start+remove:]
				lower = lower[:start] + lower[start+remove:]
			}
		}
	}
	return s
}

// stripTags removes anything between '<' and the next '>'. Unbalanced '<' with
// no matching '>' is treated as literal text.
func stripTags(s string) string {
	if !strings.ContainsRune(s, '<') {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == '<' {
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				// No closing '>': treat the rest as literal.
				out.WriteString(s[i:])
				break
			}
			i += end + 1
			continue
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}

func isASCIISpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// namedEntities is the small set of named HTML entities likely to appear in
// ATS-supplied content. The full HTML5 named-entity table is large; we keep
// only what's common and pass unknown references through verbatim.
var namedEntities = map[string]string{
	"amp":    "&",
	"lt":     "<",
	"gt":     ">",
	"quot":   "\"",
	"apos":   "'",
	"nbsp":   " ",
	"mdash":  "—",
	"ndash":  "–",
	"hellip": "…",
	"rsquo":  "’",
	"lsquo":  "‘",
	"rdquo":  "”",
	"ldquo":  "“",
}

func decodeEntities(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '&' {
			out.WriteByte(s[i])
			i++
			continue
		}
		// Find the terminating ';'. Limit lookahead to keep stray '&' cheap.
		end := -1
		for j := i + 1; j < len(s) && j-i <= 10; j++ {
			if s[j] == ';' {
				end = j
				break
			}
		}
		if end < 0 {
			out.WriteByte('&')
			i++
			continue
		}
		token := s[i+1 : end] // between '&' and ';'
		if decoded, ok := decodeOneEntity(token); ok {
			out.WriteString(decoded)
			i = end + 1
			continue
		}
		// Unknown entity: pass through verbatim.
		out.WriteByte('&')
		i++
	}
	return out.String()
}

func decodeOneEntity(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	if token[0] == '#' {
		// Numeric character reference.
		var n int64
		var err error
		if len(token) >= 2 && (token[1] == 'x' || token[1] == 'X') {
			n, err = strconv.ParseInt(token[2:], 16, 32)
		} else {
			n, err = strconv.ParseInt(token[1:], 10, 32)
		}
		if err != nil || n < 0 || n > utf8.MaxRune {
			return "", false
		}
		r := rune(n)
		// Reject C0/C1 control characters (except tab, LF, CR) and DEL.
		// Postgres rejects NUL (\x00) in text columns; C0/C1 bytes are not
		// meaningful in ATS content and produce garbage in full-text search.
		// Return ("", true) so the caller consumes the entity rather than
		// passing it through verbatim.
		if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') ||
			(r >= 0x7F && r <= 0x9F) {
			return "", true
		}
		return string(r), true
	}
	if v, ok := namedEntities[token]; ok {
		return v, true
	}
	return "", false
}
