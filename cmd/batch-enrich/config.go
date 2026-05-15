package main

import (
	"flag"
	"fmt"
	"math"
	"time"
)

// PromptVersion and Model are pinned constants. They are not overridable by
// flag because classification outputs are keyed to them: changing either
// without bumping the constant would corrupt provenance in the classifications
// table. Since these are compile-time constants, runtime format validation
// is unnecessary — the literal itself enforces the constraint.
//
// batch-enrich-v3 marks the batched contract: the agent returns a
// {"results": [...]} wrapper carrying one entry per input posting_id.
// Earlier versions returned a single-posting object per call.
const (
	PromptVersion = "batch-enrich-v3"
	Model         = "claude-haiku-4-5-20251001"
)

// Config holds the resolved runtime configuration for a batch-enrich
// invocation: pinned constants plus flag-derived knobs and CLI inputs.
type Config struct {
	// Pinned constants — not overridable by flag.
	PromptVersion string
	Model         string

	// Flag-derived knobs.
	WaveSize          int
	BatchSize         int
	MaxRetries        int
	MaxParallelAgents int
	ReportFormat      string

	// AgentTimeout caps a single `claude` subprocess invocation. Zero means
	// no per-call timeout — the call inherits the run-level context only.
	AgentTimeout time.Duration
	// StripTimeout caps a single `strip-boilerplate` subprocess invocation.
	// Zero means no per-call timeout.
	StripTimeout time.Duration

	// CLI flags.
	Count int
	Focus string
	Force bool
}

// ParseFlags parses argv into a Config using the provided FlagSet. The
// FlagSet is supplied by the caller so tests can construct one without
// touching the global flag.CommandLine.
func ParseFlags(fs *flag.FlagSet, args []string) (Config, error) {
	cfg := Config{
		PromptVersion: PromptVersion,
		Model:         Model,
	}

	fs.IntVar(&cfg.Count, "count", 10, "Max postings to select for this run.")
	fs.StringVar(&cfg.Focus, "focus", "",
		"ILIKE prefilter applied to title and description. "+
			"`%` and `_` are interpreted as SQL ILIKE wildcards. "+
			"Empty string means no filter.")
	fs.BoolVar(&cfg.Force, "force", false,
		"Drop the unclassified filter and re-process matching postings.")
	fs.StringVar(&cfg.ReportFormat, "report-format", "json",
		"Report output format: json|markdown.")
	fs.IntVar(&cfg.WaveSize, "wave-size", 10, "Number of postings dispatched per wave.")
	fs.IntVar(&cfg.BatchSize, "batch-size", 5, "Number of postings classified per agent invocation.")
	fs.IntVar(&cfg.MaxRetries, "max-retries", 3, "Retry cap per posting.")
	fs.IntVar(&cfg.MaxParallelAgents, "max-parallel", 10, "Max parallel classification agents in flight.")
	fs.DurationVar(&cfg.AgentTimeout, "agent-timeout", 0,
		"Per-invocation timeout for the `claude` subprocess (e.g. 90s). 0 disables the per-call cap.")
	fs.DurationVar(&cfg.StripTimeout, "strip-timeout", 0,
		"Per-invocation timeout for the `strip-boilerplate` subprocess (e.g. 30s). 0 disables the per-call cap.")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks flag inputs that must hold before any work begins. It
// returns a wrapped error naming the failing field.
func (c Config) Validate() error {
	if c.Count < 1 {
		return fmt.Errorf("invalid --count %d: must be >= 1", c.Count)
	}
	if c.Count > math.MaxInt32 {
		return fmt.Errorf("invalid --count %d: must be <= %d", c.Count, math.MaxInt32)
	}
	if c.WaveSize < 1 {
		return fmt.Errorf("invalid --wave-size %d: must be >= 1", c.WaveSize)
	}
	if c.BatchSize < 1 {
		return fmt.Errorf("invalid --batch-size %d: must be >= 1", c.BatchSize)
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("invalid --max-retries %d: must be >= 0", c.MaxRetries)
	}
	if c.MaxParallelAgents < 1 {
		return fmt.Errorf("invalid --max-parallel %d: must be >= 1", c.MaxParallelAgents)
	}
	if c.AgentTimeout < 0 {
		return fmt.Errorf("invalid --agent-timeout %s: must be >= 0", c.AgentTimeout)
	}
	if c.StripTimeout < 0 {
		return fmt.Errorf("invalid --strip-timeout %s: must be >= 0", c.StripTimeout)
	}
	switch c.ReportFormat {
	case "json", "markdown":
	default:
		return fmt.Errorf("invalid --report-format %q: must be json or markdown", c.ReportFormat)
	}
	return nil
}
