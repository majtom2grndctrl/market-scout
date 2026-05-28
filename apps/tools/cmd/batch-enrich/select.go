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
)

// pingTimeout bounds the initial connectivity check so a hung database does
// not block startup indefinitely.
const pingTimeout = 10 * time.Second

// DB bootstrap (OpenDB), posting selection (SelectPostings), and taxonomy
// loading (LoadTaxonomy) for batch-enrich. Taxonomy methods
// (BuildCrossTableIndex, CrossTableOwner) live here alongside the loaders
// that construct the Taxonomy value. Type definitions live in types.go.

// CrossTableOwner returns the table name owning slug, or ("", false) if not found.
func (t Taxonomy) CrossTableOwner(slug string) (string, bool) {
	owner, ok := t.crossTable[slug]
	return owner, ok
}

// BuildCrossTableIndex builds the cross-table slug index from the four
// taxonomy maps and returns a new Taxonomy with crossTable populated. A slug
// that appears in more than one table keeps the first owner encountered in
// this fixed order: canonical_roles, specializations, skills, role_dimensions.
func (t Taxonomy) BuildCrossTableIndex() Taxonomy {
	idx := make(map[string]string, len(t.CanonicalRoles)+len(t.Specializations)+len(t.Skills)+len(t.RoleDimensions))
	add := func(slugs map[string]TaxonomyEntry, table string) {
		for slug := range slugs {
			if existing, exists := idx[slug]; exists {
				slog.Warn("[batch-enrich] cross-table slug collision in taxonomy",
					"slug", slug,
					"first_owner", existing,
					"skipped_owner", table,
				)
				continue
			}
			idx[slug] = table
		}
	}
	add(t.CanonicalRoles, "canonical_roles")
	add(t.Specializations, "specializations")
	add(t.Skills, "skills")
	add(t.RoleDimensions, "role_dimensions")
	t.crossTable = idx
	return t
}

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

// SelectPostings runs the unclassified-postings query (or the forced variant
// when cfg.Force is true) and returns the selected postings. When force=true,
// alreadyClassified holds the subset of selected posting IDs that already
// have at least one classifications row — used downstream to suppress
// duplicate-write surprises in re-enrichment.
func SelectPostings(ctx context.Context, pool *sql.DB, cfg Config) ([]SelectedPosting, []int64, error) {
	queries := db.New(pool)

	postings, err := selectRows(ctx, queries, cfg)
	if err != nil {
		return nil, nil, err
	}

	slog.Info("[batch-enrich] selected postings",
		"count", len(postings),
		"focus", cfg.Focus,
		"force", cfg.Force,
		"requested", cfg.Count,
	)

	if !cfg.Force || len(postings) == 0 {
		return postings, nil, nil
	}

	ids := make([]int64, 0, len(postings))
	for _, p := range postings {
		ids = append(ids, p.PostingID)
	}
	alreadyClassified, err := queries.ListClassifiedAmong(ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("checking already-classified postings: %w", err)
	}
	return postings, alreadyClassified, nil
}

// selectRows dispatches to the unclassified or forced variant of the
// generated query and normalises the nullable columns into the working type.
// The IS NOT NULL filter in the SQL guarantees DescriptionText.Valid, so we
// take its String directly; Title remains nullable in the schema and falls
// back to the empty string when absent.
func selectRows(ctx context.Context, queries *db.Queries, cfg Config) ([]SelectedPosting, error) {
	limit := int32(cfg.Count)
	if cfg.Force {
		rows, err := queries.ListUnclassifiedPostingsForced(ctx, db.ListUnclassifiedPostingsForcedParams{
			Focus:    cfg.Focus,
			RowLimit: limit,
		})
		if err != nil {
			return nil, fmt.Errorf("listing unclassified postings (forced): %w", err)
		}
		out := make([]SelectedPosting, 0, len(rows))
		for _, r := range rows {
			out = append(out, SelectedPosting{
				PostingID:       r.PostingID,
				CompanyID:       r.CompanyID,
				Title:           nullStringOr(r.Title, ""),
				DescriptionText: r.DescriptionText.String,
			})
		}
		return out, nil
	}

	rows, err := queries.ListUnclassifiedPostings(ctx, db.ListUnclassifiedPostingsParams{
		Focus:    cfg.Focus,
		RowLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing unclassified postings: %w", err)
	}
	out := make([]SelectedPosting, 0, len(rows))
	for _, r := range rows {
		out = append(out, SelectedPosting{
			PostingID:       r.PostingID,
			CompanyID:       r.CompanyID,
			Title:           nullStringOr(r.Title, ""),
			DescriptionText: r.DescriptionText.String,
		})
	}
	return out, nil
}

// LoadTaxonomy loads all four taxonomy tables into an in-memory Taxonomy and
// builds the cross-table slug index before returning.
func LoadTaxonomy(ctx context.Context, pool *sql.DB) (Taxonomy, error) {
	queries := db.New(pool)

	roles, err := queries.ListCanonicalRoles(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading canonical_roles: %w", err)
	}
	specs, err := queries.ListSpecializations(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading specializations: %w", err)
	}
	skills, err := queries.ListSkills(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading skills: %w", err)
	}
	dims, err := queries.ListRoleDimensions(ctx)
	if err != nil {
		return Taxonomy{}, fmt.Errorf("loading role_dimensions: %w", err)
	}

	t := Taxonomy{
		CanonicalRoles:  make(map[string]TaxonomyEntry, len(roles)),
		Specializations: make(map[string]TaxonomyEntry, len(specs)),
		Skills:          make(map[string]TaxonomyEntry, len(skills)),
		RoleDimensions:  make(map[string]TaxonomyEntry, len(dims)),
	}
	for _, r := range roles {
		t.CanonicalRoles[r.Slug] = TaxonomyEntry{ID: r.ID, Name: r.Name}
	}
	for _, s := range specs {
		t.Specializations[s.Slug] = TaxonomyEntry{ID: s.ID, Name: s.Name}
	}
	for _, s := range skills {
		t.Skills[s.Slug] = TaxonomyEntry{ID: s.ID, Name: s.Name}
	}
	for _, d := range dims {
		t.RoleDimensions[d.Slug] = TaxonomyEntry{ID: d.ID, Name: d.Name}
	}
	return t.BuildCrossTableIndex(), nil
}

// nullStringOr returns ns.String when valid, otherwise the provided fallback.
func nullStringOr(ns sql.NullString, fallback string) string {
	if ns.Valid {
		return ns.String
	}
	return fallback
}
