// Command fetcher pulls job postings from configured ATS providers and
// writes append-only snapshots into Postgres.
// See: agent-context/lib/project.md
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/internal/ats"
	"github.com/majtom2grndctrl/market-scout/internal/db"
)

// perCompanyTimeout bounds the time spent fetching+writing for a single
// company so one slow ATS response cannot stall the whole run.
const perCompanyTimeout = 45 * time.Second

// maxConcurrentCompanies limits simultaneous outbound ATS fetches so we
// don't hammer downstream APIs from one cron tick.
const maxConcurrentCompanies = 5

func main() {
	if err := run(); err != nil {
		slog.Error("[fetcher] " + err.Error())
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	ctx := context.Background()

	// sqlc was generated against the database/sql driver; the pgx stdlib
	// shim gives us pgx underneath while satisfying sqlc's DBTX interface.
	pool, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer pool.Close()

	if err := pool.PingContext(ctx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}

	queries := db.New(pool)

	companies, err := queries.ListCompaniesWithATS(ctx)
	if err != nil {
		return fmt.Errorf("loading companies: %w", err)
	}

	adapters := map[string]ats.Adapter{
		"greenhouse": ats.New(nil),
	}

	var (
		wg              sync.WaitGroup
		mu              sync.Mutex
		failedCompanies []string
		successCount    int
	)

	sem := make(chan struct{}, maxConcurrentCompanies)

	for _, company := range companies {
		wg.Add(1)
		sem <- struct{}{}
		go func(c db.Company) {
			defer wg.Done()
			defer func() { <-sem }()

			fetchCtx, cancel := context.WithTimeout(ctx, perCompanyTimeout)
			defer cancel()

			if err := fetchCompany(fetchCtx, pool, adapters, c); err != nil {
				slog.Error("[fetcher] failed to fetch postings", "company", c.Name, "error", err)
				mu.Lock()
				failedCompanies = append(failedCompanies, c.Name)
				mu.Unlock()
				return
			}
			mu.Lock()
			successCount++
			mu.Unlock()
		}(company)
	}

	wg.Wait()

	total := len(companies)
	failureCount := len(failedCompanies)
	slog.Info("[fetcher] run complete",
		"total", total,
		"success", successCount,
		"failed", failureCount,
		"failed_companies", failedCompanies,
	)

	if failureCount > 0 {
		// Use os.Exit rather than returning an error so the summary log above
		// is the last line emitted, not a duplicated error message.
		os.Exit(1)
	}
	return nil
}

// fetchCompany runs the full fetch+persist cycle for one company in a
// single transaction. Either every snapshot for this run lands or none do.
func fetchCompany(ctx context.Context, pool *sql.DB, adapters map[string]ats.Adapter, c db.Company) error {
	adapter, ok := adapters[c.Ats]
	if !ok {
		return fmt.Errorf("no adapter registered for ats=%q", c.Ats)
	}

	// fetchedAt is captured before the network call so every snapshot row
	// for this run shares one timestamp, which is what trend queries key on.
	fetchedAt := time.Now().UTC()

	postings, err := adapter.FetchPostings(ctx, c.BoardToken)
	if err != nil {
		return fmt.Errorf("fetching postings: %w", err)
	}

	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	// Rollback is a no-op if Commit succeeded; safe to defer unconditionally.
	defer func() { _ = tx.Rollback() }()

	qtx := db.New(tx)

	for _, p := range postings {
		jobPostingID, err := qtx.UpsertJobPosting(ctx, db.UpsertJobPostingParams{
			CompanyID:  c.ID,
			SourceType: "ats",
			SourceUrl:  p.SourceURL,
			SourceID:   sql.NullString{String: p.SourceID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("upserting job_posting %s: %w", p.SourceURL, err)
		}

		if err := qtx.InsertPostingSnapshot(ctx, db.InsertPostingSnapshotParams{
			JobPostingID:   jobPostingID,
			FetchedAt:      fetchedAt,
			Title:          sql.NullString{String: p.Title, Valid: true},
			LocationText:   nullStr(p.LocationText),
			Department:     nullStr(p.Department),
			Team:           nullStr(p.Team),
			EmploymentType: nullStr(p.EmploymentType),
			WorkplaceType:  nullStr(p.WorkplaceType),
			PostedAt:       nullTime(p.PostedAt),
			JobUrl:         sql.NullString{String: p.JobURL, Valid: true},
			RawData:        p.RawData,
		}); err != nil {
			return fmt.Errorf("inserting snapshot for %s: %w", p.SourceURL, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	slog.Info("[fetcher] fetched postings", "company", c.Name, "count", len(postings))
	return nil
}

func nullStr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
