// Command classify is the unattended classifier. It picks existing taxonomy
// terms for each selected posting, one instrument per dimension: deterministic
// code where the evidence is a quotable phrase, the Jev decision model where
// judgment turns on context. Until the owner accepts the probe it runs only as
// a dry-run, which sends the same paid requests and writes JSONL instead of
// classifications.
// See: agent-context/plans/in-progress/hybrid-classifier/index.md
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
	"github.com/joho/godotenv"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/boilerplate"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/jev"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

const pingTimeout = 10 * time.Second

func main() {
	os.Exit(run())
}

// run is main's testable body; it returns the process exit code.
func run() int {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	cfg, err := ParseFlags(flag.NewFlagSet("classify", flag.ContinueOnError), os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] startup error: %v\n", err)
		return 2
	}

	_ = godotenv.Load(".env.local") // no-op if absent
	key := os.Getenv("AI_SERVICE_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "[classify] AI_SERVICE_KEY is not set")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// A dry-run only reads, so it connects on the read-only role.
	pool, err := openDB(ctx, "DATABASE_URL_RO")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] open db: %v\n", err)
		return 1
	}
	defer pool.Close()
	q := db.New(pool)

	outPath := cfg.OutPath
	if outPath == "" {
		outPath = filepath.Join("agent-output", "classify", "dry-run-"+time.Now().UTC().Format("2006-01-02-150405")+".jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[classify] create output dir: %v\n", err)
		return 1
	}
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] create output: %v\n", err)
		return 1
	}
	defer f.Close()

	slog.Info("[classify] starting dry-run", "prompt_version", PromptVersion, "model", cfg.Model,
		"count", cfg.Count, "concurrency", cfg.Concurrency, "out", outPath)

	loader := boilerplate.NewDBLoader(q)
	deps := runDeps{
		selectPostings: func(ctx context.Context) ([]selection.Posting, error) {
			postings, _, err := selection.Select(ctx, pool, selection.Criteria{
				Count: cfg.Count, Focus: cfg.Focus, Sort: cfg.Sort, MaxPerCompany: cfg.MaxPerCompany,
			})
			return postings, err
		},
		clean: func(ctx context.Context, companyID int64, ids []int64) ([]boilerplate.CorpusPosting, error) {
			return boilerplate.CleanSelected(ctx, loader, companyID, ids)
		},
		loadTaxonomy: func(ctx context.Context) (classify.Taxonomy, error) { return classify.LoadTaxonomy(ctx, q) },
		decider:      jev.New(key),
		now:          time.Now,
	}

	summary, err := dryRun(ctx, cfg, deps, f)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "[classify] run cancelled: %v\n", err)
			return 130
		}
		fmt.Fprintf(os.Stderr, "[classify] dry-run: %v\n", err)
		return 1
	}
	slog.Info("[classify] dry-run complete", "counts", summary.Counts, "requests", summary.Usage.Requests,
		"input_tokens", summary.Usage.Tokens, "cost_usd", summary.Usage.Cost, "interrupted", summary.Interrupted, "out", outPath)
	if summary.Interrupted {
		return 130
	}
	return 0
}

// openDB opens and pings the pool named by env.
func openDB(ctx context.Context, env string) (*sql.DB, error) {
	dsn := os.Getenv(env)
	if dsn == "" {
		return nil, fmt.Errorf("%s is not set", env)
	}
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return pool, nil
}
