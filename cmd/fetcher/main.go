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
		"greenhouse": ats.NewGreenhouse(httpClient),
		"lever":      ats.NewLever(httpClient),
		"ashby":      ats.NewAshby(httpClient),
		"workday":    ats.NewWorkday(httpClient),
		"workable":   ats.NewWorkable(httpClient),
	}

	// Partition companies up front so unknown ATS values are reported once
	// per distinct value rather than once per company. Unsupported companies
	// are expected gaps (e.g. ats='rippling' before a Rippling adapter ships), not
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
// land atomically in a single transaction, or none do. A fetch_runs row is
// inserted in its own short transaction before the adapter call so a
// mid-fetch crash leaves an observable in_progress marker; the same row is
// updated to success (inside the snapshot transaction) or failed (in a
// separate short transaction) once the work resolves.
func fetchCompany(ctx context.Context, pool *sql.DB, adapter atsAdapter, c db.Company) error {
	// fetchedAt is captured before the network call so every snapshot row
	// for this run shares one timestamp, which is what trend queries key on.
	fetchedAt := time.Now().UTC()

	// One budget covers fetch plus DB write; split only if measured evidence
	// shows one phase starving the other.
	workCtx, cancel := context.WithTimeout(ctx, companyTimeout)
	defer cancel()

	// Insert the fetch_runs marker in its own short transaction that commits
	// before the adapter call — if the process dies during the fetch, the row
	// remains visible as in_progress for postmortem.
	fetchRunID, err := insertFetchRun(workCtx, pool, c, fetchedAt)
	if err != nil {
		return fmt.Errorf("inserting fetch_run: %w", err)
	}
	slog.Info("[fetcher] inserted fetch_run", "company", c.Name, "fetch_run_id", fetchRunID)

	postings, err := adapter.FetchPostings(workCtx, c.BoardToken)
	if err != nil {
		markFetchRunFailed(ctx, pool, fetchRunID, err)
		return fmt.Errorf("fetching postings: %w", err)
	}

	tx, err := pool.BeginTx(workCtx, nil)
	if err != nil {
		markFetchRunFailed(ctx, pool, fetchRunID, err)
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
		// Missing required fields (SourceURL, SourceID, RawData) are fatal
		// adapter contract violations: persisting partial state would break
		// the board-completeness invariant. Abort the whole transaction.
		if err := validatePosting(i, p); err != nil {
			markFetchRunFailed(ctx, pool, fetchRunID, err)
			return err
		}

		jobPostingID, err := qtx.UpsertJobPosting(workCtx, db.UpsertJobPostingParams{
			CompanyID:  c.ID,
			SourceType: "ats",
			SourceUrl:  p.SourceURL,
			SourceID:   sql.NullString{String: p.SourceID, Valid: true},
		})
		if err != nil {
			markFetchRunFailed(ctx, pool, fetchRunID, err)
			return fmt.Errorf("upserting job_posting %s: %w", p.SourceURL, err)
		}

		if err := qtx.InsertPostingSnapshot(workCtx, buildSnapshotParams(jobPostingID, fetchRunID, fetchedAt, p)); err != nil {
			markFetchRunFailed(ctx, pool, fetchRunID, err)
			return fmt.Errorf("inserting snapshot for %s: %w", p.SourceURL, err)
		}
	}

	// Mark success inside the same transaction as the snapshots so the
	// fetch_runs status flip and the snapshot rows commit (or roll back)
	// atomically. postings_count fits comfortably in int32 — a board with
	// >2B postings would have other problems first. If MarkFetchRunSuccess
	// fails, the transaction rolls back (snapshots discarded) and
	// markFetchRunFailed writes the failed status outside the transaction.
	completedAt := time.Now().UTC()
	if err := qtx.MarkFetchRunSuccess(workCtx, db.MarkFetchRunSuccessParams{
		ID:            fetchRunID,
		CompletedAt:   sql.NullTime{Time: completedAt, Valid: true},
		PostingsCount: sql.NullInt32{Int32: int32(len(postings)), Valid: true},
	}); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Warn("[fetcher] rollback error", "company", c.Name, "error", rbErr)
		}
		markFetchRunFailed(ctx, pool, fetchRunID, err)
		return fmt.Errorf("marking fetch_run success: %w", err)
	}

	if err := tx.Commit(); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Warn("[fetcher] rollback error", "company", c.Name, "error", rbErr)
		}
		markFetchRunFailed(ctx, pool, fetchRunID, err)
		return fmt.Errorf("committing transaction: %w", err)
	}

	slog.Info("[fetcher] fetched postings", "company", c.Name, "count", len(postings))
	slog.Info("[fetcher] marked fetch_run success", "fetch_run_id", fetchRunID, "postings_count", len(postings))
	return nil
}

// insertFetchRun creates an in_progress fetch_runs row so it persists even
// if the rest of the work crashes. Returns the new row id. A single INSERT
// is its own implicit transaction — wrapping it in BEGIN/COMMIT would add
// two round-trips for no isolation benefit.
func insertFetchRun(ctx context.Context, pool *sql.DB, c db.Company, startedAt time.Time) (int64, error) {
	id, err := db.New(pool).InsertFetchRun(ctx, db.InsertFetchRunParams{
		CompanyID: c.ID,
		StartedAt: startedAt,
	})
	if err != nil {
		return 0, fmt.Errorf("inserting fetch run: %w", err)
	}
	return id, nil
}

// markFetchRunFailed updates the fetch_runs row to status='failed'. If the
// parent context is cancelled (shutdown), the write is skipped and the row
// is left as in_progress for the orphan reaper to handle — this preserves
// the distinction between a clean SIGTERM and a crash. Per-company timeouts
// (DeadlineExceeded on workCtx) still write failed because parentCtx remains
// uncancelled in that case.
func markFetchRunFailed(parentCtx context.Context, pool *sql.DB, fetchRunID int64, cause error) {
	if errors.Is(parentCtx.Err(), context.Canceled) {
		slog.Info("[fetcher] shutdown: skipping fetch_run failed update, leaving in_progress for orphan reaper", "fetch_run_id", fetchRunID)
		return
	}

	updateCtx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer cancel()

	completedAt := time.Now().UTC()
	queries := db.New(pool)
	if err := queries.MarkFetchRunFailed(updateCtx, db.MarkFetchRunFailedParams{
		ID:           fetchRunID,
		CompletedAt:  sql.NullTime{Time: completedAt, Valid: true},
		ErrorMessage: sql.NullString{String: cause.Error(), Valid: true},
	}); err != nil {
		slog.Warn("[fetcher] failed to mark fetch_run failed", "fetch_run_id", fetchRunID, "error", err, "original_error", cause)
		return
	}
	slog.Info("[fetcher] marked fetch_run failed", "fetch_run_id", fetchRunID, "error", cause)
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
func buildSnapshotParams(jobPostingID int64, fetchRunID int64, fetchedAt time.Time, p domain.Posting) db.InsertPostingSnapshotParams {
	return db.InsertPostingSnapshotParams{
		JobPostingID:           jobPostingID,
		FetchRunID:             sql.NullInt64{Int64: fetchRunID, Valid: true},
		FetchedAt:              fetchedAt,
		Title:                  nullStr(p.Title),
		LocationText:           nullStr(p.LocationText),
		LocationTexts:          p.LocationTexts,
		Department:             nullStr(p.Department),
		Team:                   nullStr(p.Team),
		EmploymentType:         nullStr(p.EmploymentType),
		WorkplaceType:          nullStr(p.WorkplaceType),
		PostedAt:               nullTime(p.PostedAt),
		JobUrl:                 nullStr(p.JobURL),
		RawData:                p.RawData,
		SourceFirstPublishedAt: nullTime(p.SourceFirstPublishedAt),
		SourceLastModifiedAt:   nullTime(p.SourceLastModifiedAt),
		DescriptionText:        nullStr(p.DescriptionText),
		CompensationMin:        nullInt64(p.CompensationMin),
		CompensationMax:        nullInt64(p.CompensationMax),
		CompensationCurrency:   nullStr(p.CompensationCurrency),
		CompensationPeriod:     nullStr(p.CompensationPeriod),
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

func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}
