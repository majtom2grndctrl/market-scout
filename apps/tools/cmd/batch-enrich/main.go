// Command batch-enrich classifies unclassified job postings in waves by
// dispatching parallel Haiku agents and writing back canonical role and
// skill rows. This file wires startup, signal handling, and the run flow;
// selection, dispatch, writeback, and reporting live in sibling files.
// See: agent-context/lib/ for durable architecture docs.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// runTimestampFormat is the compact timestamp embedded in each failures.jsonl
// line so an operator can correlate a record back to a specific invocation.
const runTimestampFormat = "2006-01-02-1504"

func main() {
	os.Exit(run())
}

// run is the testable body of main. It returns the desired process exit code
// so main's only job is to invoke os.Exit — keeping the boundary between
// orchestration and process control narrow.
func run() int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	fs := flag.NewFlagSet("batch-enrich", flag.ContinueOnError)
	cfg, err := ParseFlags(fs, os.Args[1:])
	if err != nil {
		// flag.ContinueOnError already printed usage on parse failure;
		// our validation errors still need to reach the operator.
		fmt.Fprintf(os.Stderr, "[batch-enrich] startup error: %v\n", err)
		return 2
	}

	logger.Info("[batch-enrich] starting batch-enrich run",
		"prompt_version", cfg.PromptVersion,
		"model", cfg.Model,
		"count", cfg.Count,
		"focus", cfg.Focus,
		"force", cfg.Force,
		"report_format", cfg.ReportFormat,
		"wave_size", cfg.WaveSize,
		"batch_size", cfg.BatchSize,
		"max_retries", cfg.MaxRetries,
		"max_parallel", cfg.MaxParallelAgents,
		"effective_concurrency", cfg.MaxParallelAgents*cfg.BatchSize,
		"progress_interval", cfg.ProgressInterval,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runTimestamp := time.Now().UTC().Format(runTimestampFormat)

	pool, err := OpenDB(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[batch-enrich] open db: %v\n", err)
		return 1
	}
	defer pool.Close()

	// Verify the claude CLI before any selection work — there is no fallback
	// if the binary is missing, so failing fast saves the operator a wasted
	// taxonomy load.
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintf(os.Stderr, "[batch-enrich] claude CLI not found in PATH: %v\n", err)
		return 1
	}

	// Verify strip-boilerplate exists too — StripBoilerplate is invoked
	// before the first wave dispatch, and a missing binary would otherwise
	// only surface after taxonomy load and selection.
	info, err := os.Stat("./bin/strip-boilerplate")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[batch-enrich] ./bin/strip-boilerplate not found — run: go build -o bin/strip-boilerplate ./cmd/strip-boilerplate: %v\n", err)
		return 1
	}
	if !info.Mode().IsRegular() {
		fmt.Fprintf(os.Stderr, "[batch-enrich] ./bin/strip-boilerplate is not a regular file\n")
		return 1
	}
	if info.Mode().Perm()&0o111 == 0 {
		fmt.Fprintf(os.Stderr, "[batch-enrich] ./bin/strip-boilerplate is not executable — run: chmod +x bin/strip-boilerplate\n")
		return 1
	}

	postings, alreadyClassified, err := SelectPostings(ctx, pool, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[batch-enrich] select postings: %v\n", err)
		return 1
	}
	selectedCount := len(postings)

	taxBefore, err := LoadTaxonomy(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[batch-enrich] load taxonomy: %v\n", err)
		return 1
	}

	runner := newClaudeRunner(cfg.Model, cfg.AgentTimeout)

	// Boilerplate stripping is corpus-wide (it groups by company), so do it
	// once up front rather than per wave.
	stripRunner := newExecStripRunner(stripBinaryPath, cfg.StripTimeout)
	stripped, err := StripBoilerplate(ctx, stripRunner, postings)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "[batch-enrich] run cancelled: %v\n", err)
			return 130
		}
		fmt.Fprintf(os.Stderr, "[batch-enrich] strip boilerplate: %v\n", err)
		return 1
	}

	// Progress reporting: pre-compute the total batch count up front so the
	// reporter can show done/total throughout the run. The reporter is
	// derived from ctx, so SIGINT/SIGTERM stops it naturally — cancellation
	// paths below don't need to cancel it separately.
	totalBatches := countBatches(len(stripped), cfg.WaveSize, cfg.BatchSize)
	tracker := &ProgressTracker{total: totalBatches, startedAt: time.Now()}
	var reporter *Reporter
	var reporterCtx context.Context
	var reporterCancel context.CancelFunc
	var reporterWG sync.WaitGroup
	if cfg.ProgressInterval > 0 {
		reporter = NewReporter(tracker, cfg.ProgressInterval, os.Stderr)
		reporterCtx, reporterCancel = context.WithCancel(ctx)
		reporterWG.Add(1)
		go func() {
			defer reporterWG.Done()
			reporter.Run(reporterCtx)
		}()
	} else {
		// Provide a no-op cancel so the normal-exit drain block can call
		// it unconditionally without a nil check.
		reporterCancel = func() {}
	}
	// Ensure the reporter goroutine is always stopped. The normal-exit path
	// below calls reporterCancel explicitly before EmitReport so the final
	// progress line clears before the report writes; this defer covers the
	// error-return and SIGTERM paths where that explicit cancel is never reached.
	defer reporterCancel()

	// Wave loop: dispatch → writeback → reload taxonomy, repeated per wave.
	// Reload taxonomy between waves so newly minted slugs from wave N's writeback
	// appear in wave N+1's prompt. Without this, a slug coined in wave 1 would
	// trigger a cross-table collision hint in wave 2 rather than being reused.
	//
	// Config.Validate enforces WaveSize >= 1, so no guard needed here.
	taxonomy := taxBefore
	results := make([]PostingResult, 0, len(stripped))

	for start := 0; start < len(stripped); start += cfg.WaveSize {
		end := start + cfg.WaveSize
		if end > len(stripped) {
			end = len(stripped)
		}
		wave := stripped[start:end]

		waveResults := RunWave(ctx, wave, taxonomy, cfg, runner, tracker)
		// WriteBack uses the current taxonomy snapshot for the role_dimensions
		// lookup; role_dimensions is a closed set never minted by agents, so
		// any reload-era snapshot is sufficient. Newly minted roles/specs/skills
		// are upserted by writeback itself and surface in the next reload.
		waveResults = WriteBack(ctx, waveResults, pool, cfg, taxonomy)
		results = append(results, waveResults...)

		if err := ctx.Err(); err != nil {
			reporterCancel()
			reporterWG.Wait()
			fmt.Fprintf(os.Stderr, "[batch-enrich] run cancelled: %v\n", err)
			return 130
		}

		// Only reload if there's a next wave to feed. The post-run report
		// uses its own taxAfter snapshot below.
		if end < len(stripped) {
			reloaded, err := LoadTaxonomy(ctx, pool)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					reporterCancel()
					reporterWG.Wait()
					fmt.Fprintf(os.Stderr, "[batch-enrich] run cancelled: %v\n", err)
					return 130
				}
				fmt.Fprintf(os.Stderr, "[batch-enrich] reload taxonomy between waves: %v\n", err)
				return 1
			}
			taxonomy = reloaded
		}
	}

	// Guard against a SIGTERM that arrived after the final wave's WriteBack
	// returned but before the report block. The spec requires no report on
	// cancellation and a non-zero exit.
	if err := ctx.Err(); err != nil {
		reporterCancel()
		reporterWG.Wait()
		fmt.Fprintf(os.Stderr, "[batch-enrich] run cancelled: %v\n", err)
		return 130
	}

	// Stop the progress reporter on the normal-exit path and emit a final
	// summary line. Cancellation paths above let the reporter drain
	// naturally via ctx — no summary on cancel.
	reporterCancel()
	reporterWG.Wait()
	if cfg.ProgressInterval > 0 {
		reporter.EmitSummary(totalBatches, time.Since(tracker.startedAt))
	}

	// Reload after the final wave so any slugs writeback minted in the last
	// wave show up under new_taxonomy in the report.
	taxAfter, err := LoadTaxonomy(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[batch-enrich] reload taxonomy for report: %v\n", err)
		return 1
	}

	report := BuildReport(cfg, selectedCount, results, taxBefore, taxAfter, alreadyClassified)
	if err := EmitReport(os.Stdout, report, cfg.ReportFormat); err != nil {
		fmt.Fprintf(os.Stderr, "[batch-enrich] emit report: %v\n", err)
		return 1
	}

	if err := AppendFailures(results, runTimestamp, cfg); err != nil {
		// The report has already been emitted — a failures-file write
		// failure is loggable but not fatal to the run's exit code.
		slog.Error("[batch-enrich] append failures", "error", err)
	}

	return 0
}
