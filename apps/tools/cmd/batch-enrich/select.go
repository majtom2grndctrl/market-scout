package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/classify"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

// pingTimeout bounds the initial connectivity check so a hung database does
// not block startup indefinitely.
const pingTimeout = 10 * time.Second

// DB bootstrap (OpenDB), posting selection (SelectPostings), and taxonomy
// loading (LoadTaxonomy, delegating to classify.LoadTaxonomy) for batch-enrich.
// Type aliases live in types.go.

// OpenDB reads DATABASE_URL, opens a pgx-backed *sql.DB pool, and pings it.
// Callers own the returned pool and must Close it. .env.local is loaded on a
// best-effort basis so local dev mirrors cmd/fetcher.
func OpenDB(ctx context.Context) (*sql.DB, error) {
	_ = godotenv.Load(".env.local") // no-op if absent; prod sets env vars directly

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is not set")
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

// SelectPostings runs the shared selection core (internal/enrich/selection) and
// adapts its results into the batch-enrich working type. The selection rules —
// the unclassified-postings query, the --force variant, and the ILIKE focus
// prefilter — live in the shared package so cmd/mcp's enrichment_preview reuses
// them unchanged. When force=true, alreadyClassified holds the subset of selected
// posting IDs that already have at least one classifications row, used downstream
// to suppress duplicate-write surprises in re-enrichment.
func SelectPostings(ctx context.Context, pool *sql.DB, cfg Config) ([]SelectedPosting, []int64, error) {
	postings, alreadyClassified, err := selection.Select(ctx, pool, selection.Criteria{
		Count: cfg.Count,
		Focus: cfg.Focus,
		Force: cfg.Force,
	})
	if err != nil {
		return nil, nil, err
	}

	slog.Info("[batch-enrich] selected postings",
		"count", len(postings),
		"focus", cfg.Focus,
		"force", cfg.Force,
		"requested", cfg.Count,
	)

	out := make([]SelectedPosting, 0, len(postings))
	for _, p := range postings {
		out = append(out, SelectedPosting{
			PostingID:       p.PostingID,
			CompanyID:       p.CompanyID,
			Title:           p.Title,
			DescriptionText: p.DescriptionText,
		})
	}
	return out, alreadyClassified, nil
}

// LoadTaxonomy loads all four taxonomy tables into an in-memory Taxonomy and
// builds the cross-table slug index. It delegates to the shared loader in
// internal/enrich/classify so batch-enrich and the MCP save_enrichment action
// build the snapshot identically.
func LoadTaxonomy(ctx context.Context, pool *sql.DB) (Taxonomy, error) {
	return classify.LoadTaxonomy(ctx, db.New(pool))
}
