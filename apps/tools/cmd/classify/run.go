package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/boilerplate"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/skillseed"
)

// runDeps are the run's inputs from outside the process. main wires them to
// the database and the Jev client; tests wire fakes.
type runDeps struct {
	selectPostings func(ctx context.Context) ([]selection.Posting, error)
	clean          func(ctx context.Context, companyID int64, ids []int64) ([]boilerplate.CorpusPosting, error)
	loadTaxonomy   func(ctx context.Context) (classify.Taxonomy, error)
	crossTable     func(ctx context.Context) ([]string, error)
	seed           skillseed.Seed
	dups           duplicateFinder
	decider        decider
	now            func() time.Time
}

// JSONL record types. The first line is the run header, one line follows per
// selected posting in completion order, and a summary line closes the file.
type runHeader struct {
	Type           string     `json:"type"`
	DryRun         bool       `json:"dry_run"`
	PromptVersion  string     `json:"prompt_version"`
	RequestedModel string     `json:"requested_model"`
	StartedAt      time.Time  `json:"started_at"`
	Thresholds     thresholds `json:"thresholds"`
	Selected       int        `json:"selected"`
	RoleOptions    int        `json:"role_options"`
	RoleChunks     int        `json:"role_chunks"`
	SpecOptions    int        `json:"specialization_options"`
	SkillNoul      int        `json:"skill_noul_options"`
	SkillLexical   int        `json:"skill_lexical_entries"`
	SeedHash       string     `json:"seed_hash"`
}

type postingLine struct {
	Type string `json:"type"`
	postingResult
}

type runSummary struct {
	Type        string          `json:"type"`
	FinishedAt  time.Time       `json:"finished_at"`
	Interrupted bool            `json:"interrupted"`
	Counts      map[outcome]int `json:"counts"`
	Usage       runUsage        `json:"usage"`
}

// dryRun runs the real pipeline over the selected postings and writes JSONL to
// w. It writes nothing to the database.
func dryRun(ctx context.Context, cfg Config, deps runDeps, w io.Writer) (runSummary, error) {
	started := deps.now()

	// Options are read once, before selection, so a role minted after this
	// point is not offered anywhere in the run.
	tax, err := deps.loadTaxonomy(ctx)
	if err != nil {
		return runSummary{}, fmt.Errorf("loading taxonomy: %w", err)
	}
	cross, err := deps.crossTable(ctx)
	if err != nil {
		return runSummary{}, fmt.Errorf("loading cross-table slugs: %w", err)
	}
	opts, err := buildOptions(tax, cross, deps.seed)
	if err != nil {
		return runSummary{}, fmt.Errorf("building options: %w", err)
	}

	selected, err := deps.selectPostings(ctx)
	if err != nil {
		return runSummary{}, fmt.Errorf("selecting postings: %w", err)
	}
	postings, preFailed, err := cleanByCompany(ctx, deps.clean, selected)
	if err != nil {
		return runSummary{}, err
	}

	out := &jsonlWriter{w: w}
	if err := out.write(runHeader{
		Type: "run", DryRun: true, PromptVersion: PromptVersion, RequestedModel: cfg.Model,
		StartedAt: started, Thresholds: pinnedThresholds, Selected: len(selected),
		RoleOptions: len(opts.roles.byName), RoleChunks: len(opts.roles.chunks),
		SpecOptions: len(opts.specs), SkillNoul: len(opts.skills),
		SkillLexical: len(opts.seed.Entries) - len(opts.skills), SeedHash: opts.seed.Hash(),
	}); err != nil {
		return runSummary{}, err
	}

	shared := newSharedDecider(deps.decider)
	c := &classifier{decider: shared, model: cfg.Model, opts: opts, dups: deps.dups, thresholds: pinnedThresholds}
	counts := map[outcome]int{}
	var mu sync.Mutex
	record := func(r postingResult) error {
		mu.Lock()
		counts[r.Outcome]++
		mu.Unlock()
		return out.write(postingLine{Type: "posting", postingResult: r})
	}

	for _, r := range preFailed {
		if err := record(r); err != nil {
			return runSummary{}, err
		}
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Concurrency)
	for _, p := range postings {
		// g.Go blocks for a free slot without watching the context, so check
		// it here: a SIGTERM stops dispatch instead of draining the queue.
		if gctx.Err() != nil {
			break
		}
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			res := c.classify(gctx, p)
			if gctx.Err() != nil {
				// A posting cut off by shutdown has no honest outcome.
				return gctx.Err()
			}
			return record(res)
		})
	}
	waitErr := g.Wait()

	summary := runSummary{
		Type: "summary", FinishedAt: deps.now(), Counts: counts, Usage: shared.totals(),
		Interrupted: ctx.Err() != nil,
	}
	if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
		return summary, waitErr
	}
	if err := out.write(summary); err != nil {
		return summary, err
	}
	return summary, nil
}

// cleanByCompany strips boilerplate one company at a time, the unit the
// stripper's prevalence rule works over. A company whose selection the
// stripper rejects fails its postings; the rest of the run proceeds.
func cleanByCompany(ctx context.Context, clean func(context.Context, int64, []int64) ([]boilerplate.CorpusPosting, error), selected []selection.Posting) ([]posting, []postingResult, error) {
	var order []int64
	byCompany := map[int64][]selection.Posting{}
	for _, p := range selected {
		if _, ok := byCompany[p.CompanyID]; !ok {
			order = append(order, p.CompanyID)
		}
		byCompany[p.CompanyID] = append(byCompany[p.CompanyID], p)
	}

	var out []posting
	var failed []postingResult
	for _, companyID := range order {
		group := byCompany[companyID]
		ids := make([]int64, len(group))
		for i, p := range group {
			ids[i] = p.PostingID
		}
		cleaned, err := clean(ctx, companyID, ids)
		var invalid *boilerplate.InvalidSelectionError
		switch {
		case errors.As(err, &invalid):
			for _, p := range group {
				failed = append(failed, postingResult{PostingID: p.PostingID, CompanyID: companyID, Title: p.Title,
					Outcome: outcomeFailed, Reason: "boilerplate: " + string(invalid.Issues[0].Code)})
			}
			continue
		case err != nil:
			return nil, nil, fmt.Errorf("stripping boilerplate for company %d: %w", companyID, err)
		}
		text := make(map[int64]string, len(cleaned))
		for _, c := range cleaned {
			text[c.PostingID] = c.DescriptionText
		}
		for _, p := range group {
			out = append(out, posting{ID: p.PostingID, CompanyID: companyID, Title: p.Title, Description: text[p.PostingID]})
		}
	}
	return out, failed, nil
}

// jsonlWriter serializes concurrent writers onto one line-oriented stream.
type jsonlWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (j *jsonlWriter) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding JSONL record: %w", err)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, err := j.w.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("writing JSONL record: %w", err)
	}
	return nil
}
