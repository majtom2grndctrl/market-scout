package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

// PromptVersion is the unattended classifier's lineage pin
// (developer-guide.md §6.2). Bump it by hand for any change to rules,
// thresholds, or question wording. A new dated Jev model does not bump it; the
// model column records that.
const PromptVersion = "classify-v1"

// DefaultModel is the Jev slug requested. Responses report the dated id they
// were served by, and that id is what provenance records.
const DefaultModel = "typesafe/jev-1.13"

// thresholds are the label rules pinned under PromptVersion. Values are the
// probe's starting points (plan.md, Delegated answers); the probe sets the
// final ones.
type thresholds struct {
	// RoleFloor is the pass-2 probability the top role must reach to be
	// written. Below it the posting is deferred.
	RoleFloor float64 `json:"role_floor"`
	// Specializations and Skills keep noul labels; Skills covers only the
	// model-judged skills, since lexical matches carry no threshold.
	Specializations labelRule `json:"specializations"`
	Skills          labelRule `json:"skills"`
	// SeniorityFloor is the probability a candidate phrase must reach to be
	// judged acting as a level.
	SeniorityFloor float64 `json:"seniority_floor"`
	// RecordingFloor is the lowest candidate probability stored as evidence.
	// Re-derive cannot move a floor below it.
	RecordingFloor float64 `json:"recording_floor"`
}

var pinnedThresholds = thresholds{
	RoleFloor:       0.40,
	Specializations: labelRule{Floor: 0.50, Relative: 0.5, Cap: 5},
	Skills:          labelRule{Floor: 0.50, Relative: 0.5, Cap: 8},
	SeniorityFloor:  0.50,
	RecordingFloor:  0.05,
}

// Config is the parsed command line.
type Config struct {
	DryRun        bool
	Count         int
	Focus         string
	Sort          selection.Sort
	MaxPerCompany int
	Concurrency   int
	Model         string
	OutPath       string
	SeedPath      string
}

// ParseFlags parses and validates the command line.
func ParseFlags(fs *flag.FlagSet, args []string) (Config, error) {
	var cfg Config
	var sort string
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "run the full pipeline and write JSONL instead of classifications (paid: sends real Jev requests)")
	fs.IntVar(&cfg.Count, "count", 10, "postings to select")
	fs.StringVar(&cfg.Focus, "focus", "", "ILIKE prefilter on title and description")
	fs.StringVar(&sort, "sort", "newest", "first_seen_at order: newest or oldest")
	fs.IntVar(&cfg.MaxPerCompany, "max-per-company", 0, "per-company cap (0 = selection default, -1 = none)")
	fs.IntVar(&cfg.Concurrency, "concurrency", 8, "postings classified at once")
	fs.StringVar(&cfg.Model, "model", DefaultModel, "Jev model slug to request")
	fs.StringVar(&cfg.OutPath, "out", "", "dry-run JSONL path (default agent-output/classify/dry-run-<timestamp>.jsonl)")
	fs.StringVar(&cfg.SeedPath, "seed", "", "skill seed file from `classify seed` (required until the seed table lands)")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	switch strings.ToLower(sort) {
	case "newest":
		cfg.Sort = selection.SortNewestFirst
	case "oldest":
		cfg.Sort = selection.SortOldestFirst
	default:
		return Config{}, fmt.Errorf("--sort must be newest or oldest, got %q", sort)
	}
	if !cfg.DryRun {
		// Live writes arrive in phase 2, after the owner accepts the probe
		// (plan.md, Owner gate). Until then there is no write path to run.
		return Config{}, errors.New("only --dry-run is available until the probe is accepted")
	}
	if cfg.SeedPath == "" {
		return Config{}, errors.New("--seed is required: generate one with `go run ./cmd/classify seed`")
	}
	if cfg.Count < 1 {
		return Config{}, fmt.Errorf("--count must be at least 1, got %d", cfg.Count)
	}
	if cfg.Concurrency < 1 {
		return Config{}, fmt.Errorf("--concurrency must be at least 1, got %d", cfg.Concurrency)
	}
	return cfg, nil
}

// SeedConfig is the parsed `classify seed` command line.
type SeedConfig struct {
	OutPath    string
	CorpusDocs int
}

// ParseSeedFlags parses the seed subcommand.
func ParseSeedFlags(fs *flag.FlagSet, args []string) (SeedConfig, error) {
	var cfg SeedConfig
	fs.StringVar(&cfg.OutPath, "out", "agent-output/classify/skill-seed.json", "where to write the generated seed")
	fs.IntVar(&cfg.CorpusDocs, "corpus", 100000, "most recently classified postings to measure name distinctiveness over (default covers all)")
	if err := fs.Parse(args); err != nil {
		return SeedConfig{}, err
	}
	if cfg.CorpusDocs < 1 {
		return SeedConfig{}, fmt.Errorf("--corpus must be at least 1, got %d", cfg.CorpusDocs)
	}
	return cfg, nil
}
