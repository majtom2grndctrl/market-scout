package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// labelRule turns a noul dimension's candidates into assigned labels. A label
// is kept when its probability clears Floor, reaches Relative times the top
// label's, and ranks within Cap. Values are tuned for precision on the gold
// set, not for historical label density, and zero survivors is a valid result.
type labelRule struct {
	Floor    float64 `json:"floor"`
	Relative float64 `json:"relative"`
	Cap      int     `json:"cap"`
}

// apply returns the kept labels, most probable first. Past the cap, the least
// probable go first.
func (r labelRule) apply(cands []label) []label {
	sorted := slices.Clone(cands)
	slices.SortFunc(sorted, byPreference)
	var kept []label
	for i, c := range sorted {
		if i >= r.Cap || c.Probability < r.Floor || c.Probability < r.Relative*sorted[0].Probability {
			break
		}
		kept = append(kept, c)
	}
	return kept
}

// recorded returns the candidates stored as evidence: every one at or above
// the recording floor, kept or not, so a later re-derive can move a threshold
// either way. Anything below is gone for good.
func recorded(cands []label, floor float64) []label {
	var out []label
	for _, c := range cands {
		if c.Probability >= floor {
			out = append(out, c)
		}
	}
	return out
}

// byPreference orders labels for keeping: quotable evidence (lexical, rule)
// before a model judgment, then by probability, then by slug for stability.
func byPreference(a, b label) int {
	if pa, pb := a.Method == methodJev, b.Method == methodJev; pa != pb {
		if pb {
			return -1
		}
		return 1
	}
	if c := cmp.Compare(b.Probability, a.Probability); c != 0 {
		return c
	}
	return cmp.Compare(a.Slug, b.Slug)
}

// saveDupThreshold mirrors c_payload_dup_at in mcp.save_enrichment (000043):
// two terms in one payload array at or above it are one concept twice, and the
// save keeps only one. The tool drops the less preferred side first, so the
// save's own choice, which ranks by usage rather than probability, never runs.
const saveDupThreshold = "0.85"

// duplicateFinder reports which pairs among terms the save would treat as
// near-duplicates. Indexes are 0-based positions in terms.
type duplicateFinder interface {
	nearDuplicatePairs(ctx context.Context, terms []label) ([][2]int, error)
}

// dropNearDuplicates keeps the preferred side of every near-duplicate pair.
// Like the save's own pass it is greedy over a total order: visit labels by
// preference and drop any that pairs with one already kept.
func dropNearDuplicates(ctx context.Context, f duplicateFinder, labels []label) ([]label, error) {
	if len(labels) < 2 {
		return labels, nil
	}
	ordered := slices.Clone(labels)
	slices.SortFunc(ordered, byPreference)
	pairs, err := f.nearDuplicatePairs(ctx, ordered)
	if err != nil {
		return nil, fmt.Errorf("checking near-duplicate labels: %w", err)
	}
	dup := make(map[[2]int]bool, len(pairs))
	for _, p := range pairs {
		dup[[2]int{min(p[0], p[1]), max(p[0], p[1])}] = true
	}
	var keptIdx []int
	for i := range ordered {
		if !slices.ContainsFunc(keptIdx, func(k int) bool { return dup[[2]int{k, i}] }) {
			keptIdx = append(keptIdx, i)
		}
	}
	kept := make([]label, len(keptIdx))
	for i, k := range keptIdx {
		kept[i] = ordered[k]
	}
	return kept, nil
}

// dbDuplicateFinder asks the database, so the tool's test is the save's test
// exactly: pg_trgm similarity on slug and name, not a Go reimplementation.
type dbDuplicateFinder struct {
	q interface {
		ListNearDuplicatePairs(ctx context.Context, arg db.ListNearDuplicatePairsParams) ([]db.ListNearDuplicatePairsRow, error)
	}
}

func (f dbDuplicateFinder) nearDuplicatePairs(ctx context.Context, terms []label) ([][2]int, error) {
	return f.pairsAt(ctx, terms, saveDupThreshold)
}

// pairsAt returns the pairs among terms whose slug or name similarity reaches
// threshold.
func (f dbDuplicateFinder) pairsAt(ctx context.Context, terms []label, threshold string) ([][2]int, error) {
	type term struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	in := make([]term, len(terms))
	for i, t := range terms {
		in[i] = term{Slug: t.Slug, Name: t.Name}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	rows, err := f.q.ListNearDuplicatePairs(ctx, db.ListNearDuplicatePairsParams{Terms: raw, Threshold: threshold})
	if err != nil {
		return nil, err
	}
	pairs := make([][2]int, len(rows))
	for i, r := range rows {
		pairs[i] = [2]int{int(r.LeftIndex) - 1, int(r.RightIndex) - 1}
	}
	return pairs, nil
}
