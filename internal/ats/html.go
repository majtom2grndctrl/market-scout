package ats

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// htmlToPlainText converts an HTML fragment into a flattened plain-text string.
// It drops entire <script> and <style> elements (tag, body, and closing tag),
// strips remaining tags, decodes the common named entities plus numeric (decimal
// and hex) character references, and collapses internal whitespace runs to single
// spaces with leading/trailing whitespace trimmed.
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

	// 1. Drop entire <script>…</script> and <style>…</style> elements so their
	//    body text never appears in output. Case-insensitive match on tag name.
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
				// No closing tag: drop to end of string.
				s = s[:start]
				lower = lower[:start]
			} else {
				remove := end + len(close)
				s = s[:start] + s[start+remove:]
				lower = lower[:start] + lower[start+remove:]
			}
		}
	}

	// 2. Strip remaining tags. Anything between '<' and the next '>' is dropped.
	//    Unbalanced '<' with no matching '>' is treated as literal text.
	var stripped strings.Builder
	stripped.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == '<' {
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				// No closing '>': treat the rest as literal.
				stripped.WriteString(s[i:])
				break
			}
			i += end + 1
			continue
		}
		stripped.WriteByte(c)
		i++
	}

	// 3. Decode entities.
	decoded := decodeEntities(stripped.String())

	// 4. Collapse whitespace runs to single spaces; trim ends.
	var out strings.Builder
	out.Grow(len(decoded))
	inSpace := false
	for _, r := range decoded {
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
	"amp":   "&",
	"lt":    "<",
	"gt":    ">",
	"quot":  "\"",
	"apos":  "'",
	"nbsp":  " ",
	"mdash": "—",
	"ndash": "–",
	"hellip": "…",
	"rsquo": "’",
	"lsquo": "‘",
	"rdquo": "”",
	"ldquo": "“",
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
