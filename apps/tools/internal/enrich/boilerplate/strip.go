// Package boilerplate detects and removes boilerplate text that recurs
// across a batch of job-posting descriptions (company blurbs, EEO footers,
// benefits sections, etc.) so downstream enrichment can focus on role-specific
// content.
package boilerplate

import (
	"math"
	"regexp"
	"strings"
)

const (
	minSamples    = 3
	minMatchLen   = 200
	minPrevalence = 0.6
)

// multiSpace collapses runs of two or more literal spaces left behind after a
// qualifying span is excised from the middle of a description. Descriptions
// arrive already whitespace-collapsed to single spaces (see package doc), so
// a run only appears where two removed (or a removed and an original) spaces
// landed adjacent to each other.
var multiSpace = regexp.MustCompile(` {2,}`)

// Strip removes exact substrings that recur, byte-for-byte, across a strong
// majority of the supplied descriptions and are at least minMatchLen bytes
// long, returning the cleaned strings in the original order. Inputs with no
// qualifying matches are returned byte-for-byte unchanged. If removal would
// leave any input empty or whitespace-only, that input is returned unchanged.
//
// Descriptions are plain text produced by internal/ats's HTML-to-text
// conversion, which collapses all whitespace — including the newlines left
// by stripped HTML block tags — down to single spaces. Real descriptions
// essentially never contain blank-line paragraph breaks, so boilerplate is
// detected as an arbitrary repeated span of text (a shared company-blurb
// prefix, an EEO-footer suffix, or a block embedded mid-document after a
// posting-specific job title), not as paragraphs delimited by blank lines.
func Strip(descriptions []string) []string {
	if len(descriptions) < minSamples {
		out := make([]string, len(descriptions))
		copy(out, descriptions)
		return out
	}

	n := len(descriptions)
	threshold := int(math.Ceil(minPrevalence * float64(n)))

	qualifying := findQualifyingSpans(descriptions, threshold)
	if len(qualifying) == 0 {
		out := make([]string, n)
		copy(out, descriptions)
		return out
	}

	out := make([]string, n)
	for i, d := range descriptions {
		cleaned := removeSpans(d, qualifying)
		if cleaned == d {
			// No qualifying span occurred in this input — original bytes.
			out[i] = d
			continue
		}
		if strings.TrimSpace(cleaned) == "" {
			// Empty-residue safety: return original input unchanged.
			out[i] = d
			continue
		}
		out[i] = cleaned
	}
	return out
}

// findQualifyingSpans locates exact substrings of at least minMatchLen bytes
// that appear in at least threshold of the supplied descriptions.
//
// It anchors on fixed-length (minMatchLen) windows: a single pass records,
// for every window value, which description indices contain it (map keys are
// windows sliced directly from the original strings, which Go does not copy,
// so this stays cheap). A window "qualifies" once it was observed in at
// least threshold descriptions. A second pass walks each description and
// merges every run of contiguous qualifying window positions into the
// maximal matching span — this is what turns many overlapping 200-byte
// windows covering the same shared blurb into one candidate string.
func findQualifyingSpans(descriptions []string, threshold int) []string {
	windowDocs := make(map[string]map[int]struct{})
	for i, d := range descriptions {
		if len(d) < minMatchLen {
			continue
		}
		for pos := 0; pos+minMatchLen <= len(d); pos++ {
			w := d[pos : pos+minMatchLen]
			docs, ok := windowDocs[w]
			if !ok {
				docs = make(map[int]struct{})
				windowDocs[w] = docs
			}
			docs[i] = struct{}{}
		}
	}

	qualifies := func(w string) bool {
		return len(windowDocs[w]) >= threshold
	}

	spanSet := make(map[string]struct{})
	addSpan := func(d string, start, end int) {
		start, end = trimToCleanBoundary(d, start, end)
		if end-start >= minMatchLen {
			spanSet[d[start:end]] = struct{}{}
		}
	}

	for _, d := range descriptions {
		if len(d) < minMatchLen {
			continue
		}
		start := -1
		for pos := 0; pos+minMatchLen <= len(d); pos++ {
			if qualifies(d[pos : pos+minMatchLen]) {
				if start < 0 {
					start = pos
				}
				continue
			}
			if start >= 0 {
				addSpan(d, start, pos+minMatchLen-1)
				start = -1
			}
		}
		if start >= 0 {
			addSpan(d, start, len(d))
		}
	}

	spans := make([]string, 0, len(spanSet))
	for s := range spanSet {
		spans = append(spans, s)
	}
	return spans
}

// isSpaceByte reports whether b is ASCII whitespace.
func isSpaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// trimToCleanBoundary narrows the half-open range [start, end) within d so
// neither edge falls inside a contiguous run of non-space characters that
// continues past the edge. Without this, a coincidentally shared character
// or two just past the true edge of a boilerplate block — e.g. several
// unrelated role descriptions happening to start with the same letter, or
// end with the same plural suffix — gets folded into the matched span and
// stripped from posting-specific content along with the real boilerplate.
// The conservative failure mode this produces — occasionally leaving a
// boilerplate instance unstripped when its surrounding whitespace happens to
// differ from the majority (e.g. glued directly to a preceding "!" with no
// space) rather than risking a match — is intentional: under-stripping loses
// nothing that downstream classification can't tolerate, but over-stripping
// corrupts posting-specific content.
func trimToCleanBoundary(d string, start, end int) (int, int) {
	for start < end && start > 0 && !isSpaceByte(d[start-1]) && !isSpaceByte(d[start]) {
		start++
	}
	for end > start && end < len(d) && !isSpaceByte(d[end-1]) && !isSpaceByte(d[end]) {
		end--
	}
	return start, end
}

// sortByLengthDesc returns a copy of strs sorted longest-first. Insertion
// sort is fine here — the qualifying-span list is always small (a handful of
// boilerplate blocks per company).
func sortByLengthDesc(strs []string) []string {
	sorted := make([]string, len(strs))
	copy(sorted, strs)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && len(sorted[j]) > len(sorted[j-1]); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	return sorted
}

// removeSpans returns a copy of d with every occurrence of every qualifying
// span removed, longest span first so a longer match is excised whole before
// a shorter, overlapping span is considered. findQualifyingSpans can produce
// several different-length variants of what is really the same boilerplate
// block, because trimToCleanBoundary trims each reference document's
// occurrence back to its own surrounding content — a document glued to the
// block with no separating space (see trimToCleanBoundary) may only contain
// a shorter variant than the one another, cleanly-separated document
// produced. Trying every variant, longest first, means such a document still
// gets its (shorter) match removed instead of being skipped just because the
// single longest variant happens not to occur in it. If d contains none of
// the spans, d is returned unchanged (same underlying bytes) so the caller
// can detect "no match" via equality.
func removeSpans(d string, spans []string) string {
	sorted := sortByLengthDesc(spans)

	changed := false
	for _, s := range sorted {
		for {
			idx := strings.Index(d, s)
			if idx < 0 {
				break
			}
			changed = true
			// Trim exactly one adjacent space on either side: descriptions
			// are single-space-joined prose, so removing a span bounded by
			// the collapsed word-separator space on each side would
			// otherwise leave a doubled space at the excision point.
			before := strings.TrimSuffix(d[:idx], " ")
			after := strings.TrimPrefix(d[idx+len(s):], " ")
			d = before + after
		}
	}
	if !changed {
		return d
	}
	d = multiSpace.ReplaceAllString(d, " ")
	return strings.TrimSpace(d)
}
