// Command classify is the unattended classifier. It picks existing taxonomy
// terms for each selected posting, one instrument per dimension: deterministic
// code where the evidence is a quotable phrase, the Jev decision model where
// judgment turns on context. Until the owner accepts the probe it runs only as
// a dry-run, which sends the same paid requests and writes JSONL instead of
// classifications.
//
//	classify --dry-run --seed <file> [flags]   classify selected postings
//	classify seed [--out <file>]               generate this install's skill seed (free, read-only)
//	classify sample [--count 150]              select a probe sample (free, read-only)
//	classify probe-report --gold <file> ...    score dry-runs against a gold set (free)
//	classify taxonomy-export                   write live option sets for gold labelers (free, read-only)
//	classify gold-merge [--decisions <file>]   merge two labelers; settle disagreements (free)
//
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
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/skillseed"
)

const pingTimeout = 10 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "seed":
			os.Exit(runSeed(os.Args[2:]))
		case "sample":
			os.Exit(runSample(os.Args[2:]))
		case "probe-report":
			os.Exit(runProbeReport(os.Args[2:]))
		case "taxonomy-export":
			os.Exit(runTaxonomyExport(os.Args[2:]))
		case "gold-merge":
			os.Exit(runGoldMerge(os.Args[2:]))
		}
	}
	os.Exit(run())
}

// runSeed generates the install's skill seed from its live skills and recent
// classifications. It reads only.
func runSeed(args []string) int {
	cfg, err := ParseSeedFlags(flag.NewFlagSet("classify seed", flag.ContinueOnError), args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] startup error: %v\n", err)
		return 2
	}
	ctx, stop, pool, code := openReadOnly()
	if code != 0 {
		return code
	}
	defer stop()
	defer pool.Close()

	seed, err := generateSeed(ctx, db.New(pool), int32(cfg.CorpusDocs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] generate seed: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(cfg.OutPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[classify] create output dir: %v\n", err)
		return 1
	}
	if err := seed.Save(cfg.OutPath); err != nil {
		fmt.Fprintf(os.Stderr, "[classify] %v\n", err)
		return 1
	}
	slog.Info("[classify] seed written", "out", cfg.OutPath, "hash", seed.Hash(), "live_skills", seed.Stats.LiveSkills,
		"corpus_docs", seed.Stats.CorpusDocs, "lexical", seed.Stats.Lexical, "rule_matched", seed.Stats.RuleMatched,
		"noul_head", seed.Stats.NoulHead, "unreachable", seed.Stats.Unreachable)
	return 0
}

// generateSeed reads what the generator measures. Skills whose slug also lives
// in another taxonomy table are left out: the save rejects them.
func generateSeed(ctx context.Context, q *db.Queries, corpusDocs int32) (skillseed.Seed, error) {
	cross, err := q.ListCrossTableSlugs(ctx)
	if err != nil {
		return skillseed.Seed{}, fmt.Errorf("loading cross-table slugs: %w", err)
	}
	excluded := make(map[string]bool, len(cross))
	for _, s := range cross {
		excluded[s] = true
	}
	rows, err := q.ListSkillLinkMass(ctx)
	if err != nil {
		return skillseed.Seed{}, fmt.Errorf("loading skills: %w", err)
	}
	var skills []skillseed.SkillStat
	for _, r := range rows {
		if !excluded[r.Slug] {
			skills = append(skills, skillseed.SkillStat{Slug: r.Slug, Name: r.Name, LinkMass: r.LinkMass})
		}
	}
	docs, err := q.ListSeedCorpus(ctx, corpusDocs)
	if err != nil {
		return skillseed.Seed{}, fmt.Errorf("loading corpus: %w", err)
	}
	corpus := make([]skillseed.Doc, len(docs))
	for i, d := range docs {
		labels := make(map[string]bool, len(d.SkillSlugs))
		for _, s := range d.SkillSlugs {
			labels[s] = true
		}
		corpus[i] = skillseed.Doc{Text: d.DescriptionText, Skills: labels}
	}
	return skillseed.Generate(skills, corpus, skillseed.DefaultParams, time.Now()), nil
}

// run is main's testable body; it returns the process exit code.
func run() int {
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

	seed, err := skillseed.Load(cfg.SeedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[classify] %v\n", err)
		return 2
	}

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

	selectPostings := func(ctx context.Context) ([]selection.Posting, error) {
		postings, _, err := selection.Select(ctx, pool, selection.Criteria{
			Count: cfg.Count, Focus: cfg.Focus, Sort: cfg.Sort, MaxPerCompany: cfg.MaxPerCompany,
		})
		return postings, err
	}
	if cfg.SamplePath != "" {
		ids, err := readSampleIDs(cfg.SamplePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[classify] %v\n", err)
			return 2
		}
		selectPostings = func(ctx context.Context) ([]selection.Posting, error) { return postingsByID(ctx, q, ids) }
	}
	var descriptions map[string]string
	if cfg.RoleDescPath != "" {
		if descriptions, err = readRoleDescriptions(cfg.RoleDescPath); err != nil {
			fmt.Fprintf(os.Stderr, "[classify] %v\n", err)
			return 2
		}
	}

	deps := runDeps{
		selectPostings:   selectPostings,
		clean:            dbCleaner(q),
		roleDescriptions: descriptions,
		loadTaxonomy:     func(ctx context.Context) (classify.Taxonomy, error) { return classify.LoadTaxonomy(ctx, q) },
		crossTable:       q.ListCrossTableSlugs,
		seed:             seed,
		dups:             dbDuplicateFinder{q: q},
		decider:          jev.New(key),
		now:              time.Now,
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

// openReadOnly loads .env.local, installs signal handling, and opens the
// read-only pool, for the subcommands that only read. A non-zero code means
// setup failed and was reported.
func openReadOnly() (context.Context, context.CancelFunc, *sql.DB, int) {
	_ = godotenv.Load(".env.local")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	pool, err := openDB(ctx, "DATABASE_URL_RO")
	if err != nil {
		stop()
		fmt.Fprintf(os.Stderr, "[classify] open db: %v\n", err)
		return nil, nil, nil, 1
	}
	return ctx, stop, pool, 0
}

// dbCleaner strips boilerplate against each company's full corpus.
func dbCleaner(q *db.Queries) func(context.Context, int64, []int64) ([]boilerplate.CorpusPosting, error) {
	loader := boilerplate.NewDBLoader(q)
	return func(ctx context.Context, companyID int64, ids []int64) ([]boilerplate.CorpusPosting, error) {
		return boilerplate.CleanSelected(ctx, loader, companyID, ids)
	}
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
