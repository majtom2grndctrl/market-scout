package main

import (
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

// PromptVersion and per-runner models are pinned constants. The --runner flag
// selects a supported runner and its associated model. Classification outputs
// record the prompt version and model. Pins prevent local runtime configuration
// from silently changing that record. Deliberate pin changes need review with
// the prompt/version contract. Since the pinned values are compile-time
// constants, runtime format validation is unnecessary — the literals enforce
// the constraint.
//
// PromptVersion carries its own batch-enrich-go-v<n> lineage, separate from the
// batch-enrich-v<n> pinned by the two batch-enrich SKILL.md files. It is a
// different classifier contract on a different transport — the agent returns a
// {"results": [...]} envelope this binary parses and writes through sqlc, while
// the skills classify one posting at a time and write through
// mcp.save_enrichment. The lineages had to split because they can also share a
// model: --runner claude pins claude-haiku-4-5-20251001, the same model the
// Claude skill uses, so (prompt_version, model) was not enough to tell the two
// writers apart. The 4,474 rows already stamped batch-enrich-v4 with that model
// are permanently ambiguous between them; see developer-guide §6.2. Those rows
// are history and are not relabelled.
//
// The -v4 generation is unchanged: it makes the {"results": [...]} envelope
// unconditional — the agent must wrap every response in it, including
// single-posting retries. v3 left the envelope conditional on multiple
// postings, which caused well-formed single-posting retries to be rejected by
// the parser. Bump to batch-enrich-go-v5 when this binary's contract changes;
// do not track the skills' numbering.
//
// This binary is slated for rewrite, so the pin is the whole of the change
// here — nothing else in the runner was reworked for provenance.
const (
	RunnerCodexExec = "codex-exec"
	RunnerClaude    = "claude"

	// SortNewest and SortOldest are the accepted --sort values. Newest-first
	// is the default: draining oldest-first keeps the classified set months
	// behind the market, which defeats emerging-title analysis.
	SortNewest = "newest"
	SortOldest = "oldest"

	PromptVersion  = "batch-enrich-go-v4"
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

	// Sort is the validated --sort value: SortNewest or SortOldest.
	Sort string
	// MaxPerCompany caps how many work units one company contributes to a
	// selection wave. Zero takes the shared selection core's default;
	// negative removes the cap.
	MaxPerCompany int
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
	fs.StringVar(&cfg.Sort, "sort", SortNewest,
		"Selection order by first_seen_at: newest|oldest.")
	fs.IntVar(&cfg.MaxPerCompany, "max-per-company", selection.DefaultMaxPerCompany,
		"Max work units one company may contribute to a selection wave. "+
			"Negative disables the cap.")
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
	if _, err := sortForFlag(c.Sort); err != nil {
		return err
	}
	if c.MaxPerCompany > math.MaxInt32 {
		return fmt.Errorf("invalid --max-per-company %d: must be <= %d", c.MaxPerCompany, math.MaxInt32)
	}
	return nil
}

// sortForFlag maps the --sort flag onto the shared selection core's Sort. An
// empty string — a Config built in code rather than from a FlagSet — maps to
// selection.SortDefault and lets the shared core supply the policy, so the
// default lives in exactly one place.
func sortForFlag(sort string) (selection.Sort, error) {
	switch sort {
	case "":
		return selection.SortDefault, nil
	case SortNewest:
		return selection.SortNewestFirst, nil
	case SortOldest:
		return selection.SortOldestFirst, nil
	default:
		return selection.SortDefault, fmt.Errorf("invalid --sort %q: must be %s or %s", sort, SortNewest, SortOldest)
	}
}
