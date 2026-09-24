package seniority

// Result is the resolved seniority.
type Result struct {
	// Value is one of the nine seniority values.
	Value string
	// Tag is v9's evidence tag, "step1-title" or "step1-body", or "" for
	// unknown.
	Tag string
	// Phrase is the verbatim phrase the note quotes, or "" for unknown.
	Phrase string
	// Uncovered is true when the winning phrase was an adjacent pair missing
	// from v9's compound table. v9 resolves those by free-text judgment the
	// tool does not make, so Value is unknown and the caller counts them.
	Uncovered bool
}

// Note returns v9's evidence line, seniority[<tag>]: "<verbatim phrase>", or
// "" for unknown: v9 records no note when there is no quote.
func (r Result) Note() string {
	if r.Tag == "" {
		return ""
	}
	return "seniority[" + r.Tag + "]: \"" + r.Phrase + "\""
}

// Resolve applies v9's Step 1. probs[i] is the judged probability that
// cands[i] acts as a level, and a candidate counts when it reaches floor; a
// candidate without a probability does not count. A counted title candidate
// beats every body candidate, as v9 quotes the title when both carry a level.
// Within a location the most probable wins, ties going to the earlier phrase.
// Step 2 is not implemented, so nothing counted means unknown.
// See: agent-context/plans/in-progress/hybrid-classifier/plan.md §Corrections
func Resolve(cands []Candidate, probs []float64, floor float64) Result {
	best := -1
	for i, c := range cands {
		if i >= len(probs) || probs[i] < floor {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		b := cands[best]
		switch {
		case c.Location == LocationTitle && b.Location != LocationTitle:
			best = i
		case c.Location != b.Location:
		case probs[i] > probs[best]:
			best = i
		case probs[i] == probs[best] && c.Offset < b.Offset:
			best = i
		}
	}
	if best < 0 {
		return Result{Value: Unknown}
	}
	c := cands[best]
	if c.Value == "" {
		return Result{Value: Unknown, Uncovered: true}
	}
	tag := "step1-body"
	if c.Location == LocationTitle {
		tag = "step1-title"
	}
	return Result{Value: c.Value, Tag: tag, Phrase: c.Phrase}
}
