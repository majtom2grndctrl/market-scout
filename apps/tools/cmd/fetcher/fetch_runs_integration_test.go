//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/internal/db"
	"github.com/majtom2grndctrl/market-scout/internal/domain"
)

// stubAdapter satisfies the unexported atsAdapter interface in this package
// without going through HTTP. The success path returns a fixed slice; the
// failure path returns the configured err. Both fetchCompany code branches
// (snapshot transaction and markFetchRunFailed) drive off the adapter outcome.
type stubAdapter struct {
	postings []domain.Posting
	err      error
}

func (s *stubAdapter) FetchPostings(_ context.Context, _ string) ([]domain.Posting, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.postings, nil
}

// openTestDB opens the integration database via DATABASE_URL using the same
// pgx-stdlib driver registration that production uses. Returning *sql.DB
// matches the fetchCompany signature directly so no shimming is required.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

// insertTestCompany creates a unique company row for the test and registers
// cleanup. Uniqueness via timestamp keeps parallel test runs safe and avoids
// colliding with the (ats, board_token) unique constraint.
func insertTestCompany(t *testing.T, pool *sql.DB) db.Company {
	t.Helper()
	ctx := t.Context()
	boardToken := "market-scout-fetch-runs-test-" + time.Now().UTC().Format("20060102T150405.000000000")

	var c db.Company
	err := pool.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id, name, ats, board_token
	`, "Market Scout Fetch-Runs Test", "greenhouse", boardToken).
		Scan(&c.ID, &c.Name, &c.Ats, &c.BoardToken)
	if err != nil {
		t.Fatalf("insert company: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Order matters: snapshots reference job_postings and fetch_runs;
		// fetch_runs and job_postings reference companies.
		if _, err := pool.ExecContext(cleanupCtx, `
			DELETE FROM posting_snapshots
			WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = $1)
		`, c.ID); err != nil {
			t.Logf("cleanup posting_snapshots: %v", err)
		}
		if _, err := pool.ExecContext(cleanupCtx, `DELETE FROM fetch_runs WHERE company_id = $1`, c.ID); err != nil {
			t.Logf("cleanup fetch_runs: %v", err)
		}
		if _, err := pool.ExecContext(cleanupCtx, `DELETE FROM job_postings WHERE company_id = $1`, c.ID); err != nil {
			t.Logf("cleanup job_postings: %v", err)
		}
		if _, err := pool.ExecContext(cleanupCtx, `DELETE FROM companies WHERE id = $1`, c.ID); err != nil {
			t.Logf("cleanup companies: %v", err)
		}
	})
	return c
}

// TestFetchCompany_SuccessPath_WritesFetchRunAndSnapshots exercises the
// happy-path lifecycle: insert in_progress fetch_runs row → adapter returns N
// postings → snapshot transaction commits with status='success',
// postings_count=N, and N posting_snapshots rows all carrying that fetch_run_id.
// The seam-crossing assertion is on the actual DB rows (not on the function's
// return value) because the load-bearing behavior is what gets persisted.
func TestFetchCompany_SuccessPath_WritesFetchRunAndSnapshots(t *testing.T) {
	pool := openTestDB(t)
	c := insertTestCompany(t, pool)

	title1, title2 := "Senior Designer", "Staff Engineer"
	postings := []domain.Posting{
		{
			SourceID:  "stub-1",
			SourceURL: "https://example.com/jobs/stub-1",
			Title:     &title1,
			RawData:   json.RawMessage(`{"id":"stub-1"}`),
		},
		{
			SourceID:  "stub-2",
			SourceURL: "https://example.com/jobs/stub-2",
			Title:     &title2,
			RawData:   json.RawMessage(`{"id":"stub-2"}`),
		},
	}

	if err := fetchCompany(t.Context(), pool, &stubAdapter{postings: postings}, c); err != nil {
		t.Fatalf("fetchCompany: unexpected error: %v", err)
	}

	// Exactly one fetch_runs row, status=success, postings_count=2.
	var (
		fetchRunID    int64
		status        string
		postingsCount sql.NullInt32
		errorMessage  sql.NullString
		completedAt   sql.NullTime
	)
	err := pool.QueryRowContext(t.Context(), `
		SELECT id, status, postings_count, error_message, completed_at
		FROM fetch_runs WHERE company_id = $1 ORDER BY id DESC LIMIT 1
	`, c.ID).Scan(&fetchRunID, &status, &postingsCount, &errorMessage, &completedAt)
	if err != nil {
		t.Fatalf("query fetch_runs: %v", err)
	}
	if status != "success" {
		t.Errorf("status: got %q, want %q", status, "success")
	}
	if !postingsCount.Valid || postingsCount.Int32 != int32(len(postings)) {
		t.Errorf("postings_count: got %+v, want %d", postingsCount, len(postings))
	}
	if errorMessage.Valid {
		t.Errorf("error_message: got %q, want NULL on success", errorMessage.String)
	}
	if !completedAt.Valid {
		t.Errorf("completed_at: got NULL, want non-NULL on success")
	}

	// Exactly N posting_snapshots, all linked to fetchRunID.
	var snapCount int
	if err := pool.QueryRowContext(t.Context(), `
		SELECT count(*) FROM posting_snapshots
		WHERE fetch_run_id = $1
	`, fetchRunID).Scan(&snapCount); err != nil {
		t.Fatalf("count posting_snapshots: %v", err)
	}
	if snapCount != len(postings) {
		t.Errorf("posting_snapshots linked to fetch_run_id=%d: got %d, want %d", fetchRunID, snapCount, len(postings))
	}

	// Cross-check: every snapshot belonging to this company carries the same
	// fetch_run_id — no orphan rows with NULL fetch_run_id slipped through.
	var unlinked int
	if err := pool.QueryRowContext(t.Context(), `
		SELECT count(*) FROM posting_snapshots ps
		JOIN job_postings jp ON jp.id = ps.job_posting_id
		WHERE jp.company_id = $1 AND (ps.fetch_run_id IS NULL OR ps.fetch_run_id != $2)
	`, c.ID, fetchRunID).Scan(&unlinked); err != nil {
		t.Fatalf("count unlinked snapshots: %v", err)
	}
	if unlinked != 0 {
		t.Errorf("found %d posting_snapshots for company %d with mismatched fetch_run_id", unlinked, c.ID)
	}
}

// TestFetchCompany_FailurePath_MarksFailedAndWritesNoSnapshots exercises the
// adapter-error branch: the in_progress fetch_runs row is updated to
// status='failed' with a non-empty error_message, and zero posting_snapshots
// land for that run. The DB rows are the contract; the function's returned
// error is asserted only as a smoke check.
func TestFetchCompany_FailurePath_MarksFailedAndWritesNoSnapshots(t *testing.T) {
	pool := openTestDB(t)
	c := insertTestCompany(t, pool)

	stubErr := errors.New("upstream ATS exploded")
	err := fetchCompany(t.Context(), pool, &stubAdapter{err: stubErr}, c)
	if err == nil {
		t.Fatalf("fetchCompany: got nil error, want non-nil for adapter failure")
	}

	var (
		fetchRunID    int64
		status        string
		postingsCount sql.NullInt32
		errorMessage  sql.NullString
		completedAt   sql.NullTime
	)
	row := pool.QueryRowContext(t.Context(), `
		SELECT id, status, postings_count, error_message, completed_at
		FROM fetch_runs WHERE company_id = $1 ORDER BY id DESC LIMIT 1
	`, c.ID)
	if err := row.Scan(&fetchRunID, &status, &postingsCount, &errorMessage, &completedAt); err != nil {
		t.Fatalf("query fetch_runs: %v", err)
	}
	if status != "failed" {
		t.Errorf("status: got %q, want %q", status, "failed")
	}
	if !errorMessage.Valid || errorMessage.String == "" {
		t.Errorf("error_message: got %+v, want non-empty", errorMessage)
	}
	if !completedAt.Valid {
		t.Errorf("completed_at: got NULL, want non-NULL on failure")
	}
	// postings_count is left NULL on failure; only success path sets it.
	if postingsCount.Valid {
		t.Errorf("postings_count: got %d, want NULL on failure", postingsCount.Int32)
	}

	// Zero snapshots linked to this fetch_run_id.
	var snapCount int
	if err := pool.QueryRowContext(t.Context(), `
		SELECT count(*) FROM posting_snapshots WHERE fetch_run_id = $1
	`, fetchRunID).Scan(&snapCount); err != nil {
		t.Fatalf("count posting_snapshots: %v", err)
	}
	if snapCount != 0 {
		t.Errorf("posting_snapshots linked to failed fetch_run_id=%d: got %d, want 0", fetchRunID, snapCount)
	}
}
