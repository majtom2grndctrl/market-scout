package main

import (
	"context"
	"fmt"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/jev"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/seniority"
)

// seniorityCandidate is one judged level phrase, kept whole so a re-derive can
// re-resolve seniority under a new floor without asking again.
type seniorityCandidate struct {
	Phrase      string             `json:"phrase"`
	Location    seniority.Location `json:"location"`
	Offset      int                `json:"offset"`
	Value       string             `json:"value"`
	Probability float64            `json:"probability"`
}

// seniorityResult is the resolved value plus the judgments behind it.
type seniorityResult struct {
	Result     seniority.Result
	Candidates []seniorityCandidate
	Model      string
	RequestKey string
}

// judgeSeniority asks one noul per distinct candidate phrase in context.
// Seniority reads the title, as v9's Step 1 does, so it gets its own request:
// the role question's request never carries the title in any form.
func judgeSeniority(ctx context.Context, d *sharedDecider, model string, p posting, floor float64) (seniorityResult, error) {
	cands := seniority.Extract(p.Title, p.Description)
	if len(cands) == 0 {
		return seniorityResult{Result: seniority.Result{Value: seniority.Unknown}}, nil
	}

	// The same phrase in the same sentence is one question.
	ids := make([]string, len(cands))
	byText := map[string]string{}
	qs := map[string]jev.Question{}
	for i, c := range cands {
		text := seniorityInstructions(c.Phrase, c.Context)
		id, ok := byText[text]
		if !ok {
			id = fmt.Sprintf("seniority_%03d", len(byText)+1)
			byText[text] = id
			qs[id] = jev.Question{Type: jev.TypeNoul, Instructions: text}
		}
		ids[i] = id
	}
	resp, key, err := d.decide(ctx, jev.Request{Model: model, State: seniorityState(p), Questions: qs})
	if err != nil {
		return seniorityResult{}, fmt.Errorf("seniority: %w", err)
	}

	probs := make([]float64, len(cands))
	judged := make([]seniorityCandidate, len(cands))
	for i, c := range cands {
		probs[i] = *resp.Answers[ids[i]].Noul
		judged[i] = seniorityCandidate{Phrase: c.Phrase, Location: c.Location, Offset: c.Offset, Value: c.Value, Probability: probs[i]}
	}
	return seniorityResult{Candidates: judged, Model: resp.Model, RequestKey: key,
		Result: seniority.Resolve(cands, probs, floor)}, nil
}

// resolveStored re-applies Step 1 to stored judgments under a floor.
func resolveStored(stored []seniorityCandidate, floor float64) seniority.Result {
	cands := make([]seniority.Candidate, len(stored))
	probs := make([]float64, len(stored))
	for i, s := range stored {
		cands[i] = seniority.Candidate{Phrase: s.Phrase, Location: s.Location, Offset: s.Offset, Value: s.Value}
		probs[i] = s.Probability
	}
	return seniority.Resolve(cands, probs, floor)
}
