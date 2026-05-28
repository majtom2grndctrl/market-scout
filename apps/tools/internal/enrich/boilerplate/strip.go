// Package boilerplate detects and removes boilerplate paragraphs that recur
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

// paragraphSplit matches a run of two or more newlines, optionally separated
// by whitespace-only lines. Used both to split text into paragraphs and to
// collapse stray blank-line runs left after paragraph removal.
var paragraphSplit = regexp.MustCompile(`\n[ \t]*(?:\n[ \t]*)+`)

// Strip removes paragraphs that appear in a strong majority of the supplied
// descriptions and exceed a minimum length, returning the cleaned strings in
// the original order. Inputs with no qualifying matches are returned
// byte-for-byte unchanged. If removal would leave any input empty or
// whitespace-only, that input is returned unchanged.
func Strip(descriptions []string) []string {
	if len(descriptions) < minSamples {
		out := make([]string, len(descriptions))
		copy(out, descriptions)
		return out
	}

	n := len(descriptions)
	threshold := int(math.Ceil(minPrevalence * float64(n)))

	// Per-input normalized paragraphs and the unique set of paragraphs each
	// input contains.
	paragraphsPerInput := make([][]string, n)
	uniquePerInput := make([]map[string]struct{}, n)
	for i, d := range descriptions {
		paras := splitParagraphs(d)
		paragraphsPerInput[i] = paras
		set := make(map[string]struct{}, len(paras))
		for _, p := range paras {
			set[p] = struct{}{}
		}
		uniquePerInput[i] = set
	}

	// Count, by input, how many inputs contain each paragraph.
	counts := make(map[string]int)
	for _, set := range uniquePerInput {
		for p := range set {
			counts[p]++
		}
	}

	// Filter to qualifying paragraphs: prevalence and length.
	qualifying := make([]string, 0)
	for p, c := range counts {
		if c >= threshold && len(p) >= minMatchLen {
			qualifying = append(qualifying, p)
		}
	}

	if len(qualifying) == 0 {
		out := make([]string, n)
		copy(out, descriptions)
		return out
	}

	// Overlap handling: if paragraph A is a substring of paragraph B (and
	// they differ), drop A in favor of B.
	qualifying = dropSubstrings(qualifying)

	qualifyingSet := make(map[string]struct{}, len(qualifying))
	for _, p := range qualifying {
		qualifyingSet[p] = struct{}{}
	}

	out := make([]string, n)
	for i, d := range descriptions {
		paras := paragraphsPerInput[i]
		// Determine whether this input contains any qualifying paragraph.
		hasMatch := false
		for _, p := range paras {
			if _, ok := qualifyingSet[p]; ok {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			// No modification — return original bytes.
			out[i] = d
			continue
		}

		cleaned := removeQualifying(d, paras, qualifyingSet)
		if strings.TrimSpace(cleaned) == "" {
			// Empty-residue safety: return original input unchanged.
			out[i] = d
			continue
		}
		out[i] = cleaned
	}
	return out
}

// splitParagraphs normalizes line endings and splits the input on runs of two
// or more newlines (allowing whitespace-only blank lines between them).
// Empty paragraphs are dropped. Intra-paragraph whitespace is preserved.
func splitParagraphs(s string) []string {
	normalized := normalizeNewlines(s)
	parts := paragraphSplit.Split(normalized, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// normalizeNewlines converts CRLF and lone CR sequences to LF.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// dropSubstrings removes any paragraph that is a strict substring of another
// qualifying paragraph, preferring the longest. Sorting longest-first means a
// shorter paragraph contained in any longer one is filtered out.
func dropSubstrings(paragraphs []string) []string {
	// Sort by descending length so we always check shorter against longer.
	sorted := make([]string, len(paragraphs))
	copy(sorted, paragraphs)
	// Simple insertion sort by length desc — paragraph counts here are small.
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && len(sorted[j]) > len(sorted[j-1]); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	kept := make([]string, 0, len(sorted))
	for _, p := range sorted {
		contained := false
		for _, k := range kept {
			if len(p) < len(k) && strings.Contains(k, p) {
				contained = true
				break
			}
		}
		if !contained {
			kept = append(kept, p)
		}
	}
	return kept
}

// removeQualifying returns a cleaned version of orig with any qualifying
// paragraph removed. It works on the normalized form to ensure paragraph
// boundaries match what splitParagraphs produced; this means returned strings
// always use LF line endings.
func removeQualifying(orig string, paras []string, qualifying map[string]struct{}) string {
	normalized := normalizeNewlines(orig)

	// Walk the normalized string and rebuild it, dropping qualifying
	// paragraphs while preserving the separators between non-removed ones.
	// We locate each paragraph in order; everything between the previous
	// paragraph's end and the current paragraph's start is a separator.
	var b strings.Builder
	cursor := 0
	for _, p := range paras {
		idx := strings.Index(normalized[cursor:], p)
		if idx < 0 {
			// Invariant: every paragraph in paras was split from this normalized
			// string, so it must appear in it. A miss here means normalizeNewlines
			// produced a different result between splitParagraphs and removeQualifying
			// — check for double-normalization.
			continue
		}
		start := cursor + idx
		end := start + len(p)
		separator := normalized[cursor:start]

		if _, drop := qualifying[p]; drop {
			// Skip the paragraph; also drop the leading separator so we
			// don't accumulate orphaned blank lines. The trailing separator
			// will be handled when we encounter the next paragraph.
			cursor = end
			continue
		}

		b.WriteString(separator)
		b.WriteString(p)
		cursor = end
	}
	// Append any trailing content after the last paragraph (typically just
	// whitespace, but preserve it for the collapse/trim step).
	if cursor < len(normalized) {
		b.WriteString(normalized[cursor:])
	}

	result := b.String()
	// Collapse runs of two or more newlines (including whitespace-only blank
	// lines between them) down to a single blank line ("\n\n").
	result = paragraphSplit.ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}
