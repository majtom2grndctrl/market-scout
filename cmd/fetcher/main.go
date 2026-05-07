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
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/internal/ats"
	"github.com/majtom2grndctrl/market-scout/internal/db"
	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

// companyTimeout bounds the full per-company unit of work: HTTP fetch plus DB
// write share one budget rather than split.
const companyTimeout = 45 * time.Second

// maxConcurrentCompanies limits simultaneous outbound ATS fetches to avoid
// hammering downstream APIs.
const maxConcurrentCompanies = 5

// atsAdapter is the contract the fetcher needs from any ATS integration.
// Defined here (the consumer) rather than in internal/ats so the interface
// reflects the fetcher's needs; concrete adapters satisfy it implicitly.
type atsAdapter interface {
	FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error)
}

// errPartialFailure signals that one or more companies failed during a run.
var errPartialFailure = errors.New("one or more companies failed")

var errInvariantViolated = errors.New("summary invariant violated: goroutine returned without recording outcome")

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, errPartialFailure) {
			slog.Error("[fetcher] fatal error", "error", err)
		}
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load(".env.local") // no-op if absent; prod sets env vars directly

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// sqlc was generated against the database/sql driver; the pgx stdlib
	// shim gives us pgx underneath while satisfying sqlc's DBTX interface.
	pool, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer pool.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := pool.PingContext(pingCtx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}

	queries := db.New(pool)

	companies, err := queries.ListCompaniesWithATS(ctx)
	if err != nil {
		return fmt.Errorf("loading companies: %w", err)
	}

	httpClient := &http.Client{}
	adapters := map[string]atsAdapter{
		"greenhouse": ats.New(httpClient),
	}

	// Partition companies up front so unknown ATS values are reported once
	// per distinct value rather than once per company. Unsupported companies
	// are expected gaps (e.g. ats='lever' before a Lever adapter ships), not
	// failures, so they don't enter the success/failure tally.
	var supported []db.Company
	unsupportedByATS := make(map[string]int)
	for _, company := range companies {
		if _, ok := adapters[company.Ats]; ok {
			supported = append(supported, company)
			continue
		}
		unsupportedByATS[company.Ats]++
	}
	for atsValue, count := range unsupportedByATS {
		slog.Warn("[fetcher] no adapter registered, skipping companies", "ats", atsValue, "count", count)
	}

	var (
		wg               sync.WaitGroup
		mu               sync.Mutex
		failedCompanies  []string
		abortedCompanies []string
		successCount     int
	)

	// Buffered channel as semaphore: send to acquire a slot, receive to release.
	sem := make(chan struct{}, maxConcurrentCompanies)

	totalAttempted := len(supported)

	for _, company := range supported {
		// Acquire a worker slot, but bail on root-context cancellation — a bare
		// sem send would block indefinitely after SIGINT/SIGTERM. Undispatched
		// companies count as aborted_shutdown to preserve the summary invariant.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			mu.Lock()
			abortedCompanies = append(abortedCompanies, company.Name)
			mu.Unlock()
			continue
		}
		// safe: supported slice only contains companies with registered adapters
		a := adapters[company.Ats]
		wg.Add(1)
		go func(c db.Company, adapter atsAdapter) {
			defer wg.Done()
			defer func() { <-sem }()

			err := fetchCompany(ctx, pool, adapter, c)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
				return
			}

			// Heuristic: workCtx surfaces its own timeout as DeadlineExceeded; Canceled
			// propagates from the parent ctx (shutdown signal). The parent-context check
			// confirms the signal came from above, not from a future nested cancellation.
			if classifyCompanyError(err, ctx.Err()) {
				slog.Warn("[fetcher] aborted by shutdown", "company", c.Name, "error", err)
				mu.Lock()
				abortedCompanies = append(abortedCompanies, c.Name)
				mu.Unlock()
				return
			}

			slog.Error("[fetcher] failed to fetch postings", "company", c.Name, "error", err)
			mu.Lock()
			failedCompanies = append(failedCompanies, c.Name)
			mu.Unlock()
		}(company, a)
	}
	wg.Wait()

	failureCount := len(failedCompanies)
	abortedCount := len(abortedCompanies)
	skippedUnsupported := len(companies) - len(supported)

	// Every attempted company must land in exactly one bucket; a mismatch
	// indicates a goroutine that returned without recording its outcome.
	var invariantViolated bool
	if successCount+failureCount+abortedCount != totalAttempted {
		invariantViolated = true
		slog.Error("[fetcher] summary invariant violated",
			"success", successCount,
			"failed", failureCount,
			"aborted_shutdown", abortedCount,
			"total_attempted", totalAttempted,
		)
	}

	slog.Info("[fetcher] run complete",
		"total_attempted", totalAttempted,
		"success", successCount,
		"failed", failureCount,
		"aborted_shutdown", abortedCount,
		"skipped_unsupported", skippedUnsupported,
		"failed_companies", failedCompanies,
		"aborted_companies", abortedCompanies,
	)

	// Sentinels let deferred cleanup (pool.Close, signal stop) run; main()
	// maps them to exit 1. Shutdown alone is exit 0.
	if invariantViolated {
		return errInvariantViolated
	}
	if failureCount > 0 {
		slog.Warn("[fetcher] exiting with partial failure", "failed", failureCount)
		return errPartialFailure
	}
	return nil
}

// fetchCompany runs the fetch+persist cycle for one company: all snapshots
// land atomically in a single transaction, or none do.
func fetchCompany(ctx context.Context, pool *sql.DB, adapter atsAdapter, c db.Company) error {
	// fetchedAt is captured before the network call so every snapshot row
	// for this run shares one timestamp, which is what trend queries key on.
	fetchedAt := time.Now().UTC()

	// One budget covers fetch plus DB write; split only if measured evidence
	// shows one phase starving the other.
	workCtx, cancel := context.WithTimeout(ctx, companyTimeout)
	defer cancel()

	postings, err := adapter.FetchPostings(workCtx, c.BoardToken)
	if err != nil {
		return fmt.Errorf("fetching postings: %w", err)
	}

	tx, err := pool.BeginTx(workCtx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	// Rollback is a no-op after Commit (returns sql.ErrTxDone); safe to defer
	// unconditionally. Log unexpected errors — a real rollback failure means
	// the connection may have died mid-transaction.
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Warn("[fetcher] rollback error", "company", c.Name, "error", err)
		}
	}()

	qtx := db.New(tx)

	for i, p := range postings {
		// Empty RawData is a fatal adapter contract violation: persisting
		// the company's snapshots without it would leave the board in a
		// partial state and break the "fetched_at represents a complete
		// view of the board" invariant. Abort the whole transaction.
		if err := validatePosting(i, p); err != nil {
			return err
		}

		jobPostingID, err := qtx.UpsertJobPosting(workCtx, db.UpsertJobPostingParams{
			CompanyID:  c.ID,
			SourceType: "ats",
			SourceUrl:  p.SourceURL,
			SourceID:   sql.NullString{String: p.SourceID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("upserting job_posting %s: %w", p.SourceURL, err)
		}

		if err := qtx.InsertPostingSnapshot(workCtx, buildSnapshotParams(jobPostingID, fetchedAt, p)); err != nil {
			return fmt.Errorf("inserting snapshot for %s: %w", p.SourceURL, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	slog.Info("[fetcher] fetched postings", "company", c.Name, "count", len(postings))
	return nil
}

// classifyCompanyError returns true if the error should count as aborted_shutdown
// (parent context was cancelled) rather than a genuine fetch/write failure.
func classifyCompanyError(err error, parentCtxErr error) bool {
	return errors.Is(err, context.Canceled) && parentCtxErr != nil
}

// validatePosting returns an error if the posting violates the adapter contract
// for required fields. These are checked at the fetcher's write boundary so
// adapters don't silently persist corrupted state.
func validatePosting(i int, p domain.Posting) error {
	if p.SourceURL == "" {
		return fmt.Errorf("posting %d: empty SourceURL — adapter contract violation", i)
	}
	if p.SourceID == "" {
		return fmt.Errorf("posting %d (source_url=%s): empty SourceID — adapter contract violation", i, p.SourceURL)
	}
	if len(p.RawData) == 0 {
		return fmt.Errorf("posting %d (source_url=%s): empty RawData — adapter contract violation", i, p.SourceURL)
	}
	return nil
}

// buildSnapshotParams maps a domain.Posting onto the sqlc-generated insert
// params struct. Extracted so the wire-up between domain optional fields and
// the nullable DB columns can be unit-tested without a live database.
func buildSnapshotParams(jobPostingID int64, fetchedAt time.Time, p domain.Posting) db.InsertPostingSnapshotParams {
	return db.InsertPostingSnapshotParams{
		JobPostingID:           jobPostingID,
		FetchedAt:              fetchedAt,
		Title:                  nullStr(p.Title),
		LocationText:           nullStr(p.LocationText),
		Department:             nullStr(p.Department),
		Team:                   nullStr(p.Team),
		EmploymentType:         nullStr(p.EmploymentType),
		WorkplaceType:          nullStr(p.WorkplaceType),
		PostedAt:               nullTime(p.PostedAt),
		JobUrl:                 nullStr(p.JobURL),
		RawData:                p.RawData,
		SourceFirstPublishedAt: nullTime(p.SourceFirstPublishedAt),
		SourceLastModifiedAt:   nullTime(p.SourceLastModifiedAt),
	}
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
