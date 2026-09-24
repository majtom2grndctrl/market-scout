package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/boilerplate"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/jev"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/skillseed"
)

// fakeDecider stands in for Jev. Its answers are driven by markers in the
// state, so a test reads its intent from the posting text:
//
//   - a role name in the state gets 0.9 wherever it is offered
//   - "NONEFIT" makes none_fit win
//   - "LOW" gives the named role 0.38 and spreads the rest
//   - "FAIL" returns an error
//   - "NOUL:<slug>" answers that term's noul question 0.9, every other noul
//     0.02; "NOUL:<slug>=0.6" answers it 0.6
type fakeDecider struct {
	mu       sync.Mutex
	requests []jev.Request
	model    string
}

func newFakeDecider() *fakeDecider { return &fakeDecider{model: "typesafe/jev-1.13-20260917"} }

var errFakeJev = errors.New("fake jev: retries exhausted")

func (f *fakeDecider) Decide(_ context.Context, req jev.Request) (jev.Response, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()

	if strings.Contains(req.State, "FAIL") {
		return jev.Response{}, errFakeJev
	}
	resp := jev.Response{Model: f.model, Answers: map[string]jev.Answer{}, Usage: jev.Usage{InputTokens: 100, Cost: 0.0000042}}
	for id, q := range req.Questions {
		if q.Type == jev.TypeNoul {
			resp.Answers[id] = fakeNoul(req.State, id)
			continue
		}
		resp.Answers[id] = fakeChoice(req.State, q)
	}
	return resp, nil
}

func fakeNoul(state, id string) jev.Answer {
	_, slug, _ := strings.Cut(id, ":")
	p := 0.02
	for _, f := range strings.Fields(state) {
		term, val, hasVal := strings.Cut(strings.TrimPrefix(f, "NOUL:"), "=")
		if !strings.HasPrefix(f, "NOUL:") || term != slug {
			continue
		}
		p = 0.9
		if hasVal {
			fmt.Sscanf(val, "%g", &p)
		}
	}
	return jev.Answer{Type: jev.TypeNoul, Noul: &p}
}

func fakeChoice(state string, q jev.Question) jev.Answer {
	state = strings.Join(strings.Fields(state), " ")
	probs := make(map[string]float64, len(q.Criteria))
	var target string
	for opt := range q.Criteria {
		probs[opt] = 0
		if opt != noneFit && strings.Contains(state, opt) {
			target = opt
		}
	}
	switch {
	case strings.Contains(state, "NONEFIT") || target == "":
		probs[noneFit] = 1
	case strings.Contains(state, "LOW"):
		probs[target], probs[noneFit] = 0.38, 0.32
		var rest []string
		for opt := range q.Criteria {
			if opt != target && opt != noneFit {
				rest = append(rest, opt)
			}
		}
		if len(rest) == 0 {
			probs[noneFit] += 0.30
		}
		for _, opt := range rest {
			probs[opt] = 0.30 / float64(len(rest))
		}
	default:
		probs[target], probs[noneFit] = 0.9, 0.1
	}
	best, bestP := "", -1.0
	for opt, p := range probs {
		if p > bestP || (p == bestP && opt < best) {
			best, bestP = opt, p
		}
	}
	return jev.Answer{Type: jev.TypeChoice, Choice: best, Confidence: bestP, Probabilities: probs}
}

func (f *fakeDecider) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// taxonomyWithRoles builds n roles whose slugs and names differ, so a test can
// tell which one reached the model, plus a few specializations and skills.
func taxonomyWithRoles(n int) classify.Taxonomy {
	t := classify.Taxonomy{
		CanonicalRoles:  map[string]classify.TaxonomyEntry{},
		Specializations: map[string]classify.TaxonomyEntry{},
		Skills:          map[string]classify.TaxonomyEntry{},
	}
	for i := range n {
		t.CanonicalRoles[fmt.Sprintf("role-%03d", i)] = classify.TaxonomyEntry{ID: int64(i + 1), Name: fmt.Sprintf("Job Family %03d", i)}
	}
	for i, s := range []string{"payments", "developer-tools", "ai-agents"} {
		t.Specializations[s] = classify.TaxonomyEntry{ID: int64(i + 1), Name: s}
	}
	for i, s := range []string{"python", "terraform", "stakeholder-management", "problem-solving"} {
		t.Skills[s] = classify.TaxonomyEntry{ID: int64(i + 1), Name: s}
	}
	return t
}

// testSeed reaches python and terraform lexically and two soft skills by noul.
func testSeed() skillseed.Seed {
	return skillseed.Seed{Entries: []skillseed.Entry{
		{SkillSlug: "python", SkillName: "Python", Reach: skillseed.ReachLexical, Aliases: []string{"Python"}},
		{SkillSlug: "terraform", SkillName: "Terraform", Reach: skillseed.ReachLexical, Aliases: []string{"Terraform"}},
		{SkillSlug: "stakeholder-management", Reach: skillseed.ReachNoul},
		{SkillSlug: "problem-solving", Reach: skillseed.ReachNoul},
	}}
}

// testDeps wires a dry-run over in-memory postings. Descriptions pass through
// the cleaner unchanged.
func testDeps(tax classify.Taxonomy, d decider, postings ...selection.Posting) runDeps {
	return runDeps{
		selectPostings: func(context.Context) ([]selection.Posting, error) { return postings, nil },
		clean: func(_ context.Context, companyID int64, ids []int64) ([]boilerplate.CorpusPosting, error) {
			var out []boilerplate.CorpusPosting
			for _, id := range ids {
				for _, p := range postings {
					if p.PostingID == id {
						out = append(out, boilerplate.CorpusPosting{PostingID: id, DescriptionText: p.DescriptionText})
					}
				}
			}
			return out, nil
		},
		loadTaxonomy: func(context.Context) (classify.Taxonomy, error) { return tax, nil },
		crossTable:   func(context.Context) ([]string, error) { return nil, nil },
		seed:         testSeed(),
		dups:         &pairFinder{},
		decider:      d,
		now:          func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) },
	}
}

func testConfig() Config {
	return Config{DryRun: true, Count: 10, Concurrency: 4, Model: DefaultModel}
}

// runDry runs a dry-run and returns its posting lines by id plus the summary.
func runDry(t *testing.T, deps runDeps) (map[int64]postingResult, runSummary, []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	summary, err := dryRun(t.Context(), testConfig(), deps, &buf)
	if err != nil {
		t.Fatalf("dryRun: %v", err)
	}
	byID := map[int64]postingResult{}
	var raw []map[string]any
	sc := bufio.NewScanner(&buf)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line is not JSON: %v", err)
		}
		raw = append(raw, m)
		if m["type"] == "posting" {
			var r postingResult
			_ = json.Unmarshal(sc.Bytes(), &r)
			byID[r.PostingID] = r
		}
	}
	return byID, summary, raw
}
