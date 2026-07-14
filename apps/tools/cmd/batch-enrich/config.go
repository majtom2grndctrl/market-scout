package main

import (
	"flag"
	"fmt"
	"math"
	"time"
)

// PromptVersion and per-runner models are pinned constants. The --runner flag
// selects a supported runner and its associated model. Classification outputs
// record the prompt version and model. Pins prevent local runtime configuration
// from silently changing that record. Deliberate pin changes need review with
// the prompt/version contract. Since the pinned values are compile-time
// constants, runtime format validation is unnecessary — the literals enforce
// the constraint.
//
// batch-enrich-v4 makes the {"results": [...]} envelope unconditional — the
// agent must wrap every response in it, including single-posting retries.
// v3 left the envelope conditional on multiple postings, which caused
// well-formed single-posting retries to be rejected by the parser.
const (
	RunnerCodexExec = "codex-exec"
	RunnerClaude    = "claude"

	PromptVersion  = "batch-enrich-v4"
	CodexExecModel = "gpt-5.4-mini"
	ClaudeModel    = "claude-haiku-4-5-20251001"
)

// Config holds the resolved runtime configuration for a batch-enrich
// invocation: pinned constants plus flag-derived knobs and CLI inputs.
type Config struct {
	// PromptVersion is a pinned constant.
	PromptVersion string
	// Runner is the validated --runner selection.
	Runner string
	// Model is pinned to the selected runner.
	Model string

	// Flag-derived knobs.
	WaveSize          int
	BatchSize         int
	MaxRetries        int
	MaxParallelAgents int
	ReportFormat      string

	// AgentTimeout caps a single agent subprocess invocation. Zero means
	// no per-call timeout — the call inherits the run-level context only.
	AgentTimeout time.Duration
	// StripTimeout caps a single `strip-boilerplate` subprocess invocation.
	// Zero means no per-call timeout.
	StripTimeout time.Duration

	// ProgressInterval is how often the progress reporter emits an update
	// to stderr. Zero disables progress output entirely.
	ProgressInterval time.Duration

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
		Runner:        RunnerCodexExec,
	}

	fs.IntVar(&cfg.Count, "count", 10, "Max postings to select for this run.")
	fs.StringVar(&cfg.Focus, "focus", "",
		"ILIKE prefilter applied to title and description. "+
			"`%` and `_` are interpreted as SQL ILIKE wildcards. "+
			"Empty string means no filter.")
	fs.BoolVar(&cfg.Force, "force", false,
		"Drop the unclassified filter and re-process matching postings.")
	fs.StringVar(&cfg.Runner, "runner", RunnerCodexExec,
		"Classification runner: codex-exec|claude.")
	fs.StringVar(&cfg.ReportFormat, "report-format", "json",
		"Report output format: json|markdown.")
	fs.IntVar(&cfg.WaveSize, "wave-size", 10, "Number of postings dispatched per wave.")
	fs.IntVar(&cfg.BatchSize, "batch-size", 5, "Number of postings classified per agent invocation.")
	fs.IntVar(&cfg.MaxRetries, "max-retries", 3, "Retry cap per posting.")
	fs.IntVar(&cfg.MaxParallelAgents, "max-parallel", 10, "Max parallel classification agents in flight.")
	fs.DurationVar(&cfg.AgentTimeout, "agent-timeout", 0,
		"Per-invocation timeout for the classification subprocess (e.g. 90s). 0 disables the per-call cap.")
	fs.DurationVar(&cfg.StripTimeout, "strip-timeout", 0,
		"Per-invocation timeout for the `strip-boilerplate` subprocess (e.g. 30s). 0 disables the per-call cap.")
	fs.DurationVar(&cfg.ProgressInterval, "progress-interval", 2*time.Second,
		"How often to emit progress to stderr. 0 disables progress output.")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	model, err := modelForRunner(cfg.Runner)
	if err != nil {
		return Config{}, err
	}
	cfg.Model = model

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func modelForRunner(runner string) (string, error) {
	switch runner {
	case RunnerCodexExec:
		return CodexExecModel, nil
	case RunnerClaude:
		return ClaudeModel, nil
	default:
		return "", fmt.Errorf("invalid --runner %q: must be %s or %s", runner, RunnerCodexExec, RunnerClaude)
	}
}

// Validate checks flag inputs that must hold before any work begins. It
// returns a wrapped error naming the failing field.
func (c Config) Validate() error {
	if _, err := modelForRunner(c.Runner); err != nil {
		return err
	}
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
	if c.ProgressInterval < 0 {
		return fmt.Errorf("invalid --progress-interval %s: must be >= 0", c.ProgressInterval)
	}
	switch c.ReportFormat {
	case "json", "markdown":
	default:
		return fmt.Errorf("invalid --report-format %q: must be json or markdown", c.ReportFormat)
	}
	return nil
}
