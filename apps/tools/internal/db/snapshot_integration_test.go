//go:build integration

package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostingSnapshots_AppendsRowsWithoutUpsert(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx := t.Context()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	// Unique board_token keeps the row distinguishable from any seed data
	// and avoids colliding with the (ats, board_token) unique constraint.
	boardToken := "market-scout-test-" + time.Now().UTC().Format("20060102T150405.000000000")

	var companyID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Market Scout Test Co", "greenhouse", boardToken).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	sourceURL := "https://example.com/jobs/" + boardToken

	var jobPostingID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url)
		VALUES ($1, $2, $3)
		RETURNING id
	`, companyID, "ats", sourceURL).Scan(&jobPostingID); err != nil {
		t.Fatalf("insert job_posting: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM posting_snapshots WHERE job_posting_id = $1`, jobPostingID); err != nil {
			t.Logf("cleanup posting_snapshots: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM job_postings WHERE id = $1`, jobPostingID); err != nil {
			t.Logf("cleanup job_postings: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM companies WHERE id = $1`, companyID); err != nil {
			t.Logf("cleanup companies: %v", err)
		}
	})

	firstFetch := time.Now().UTC().Truncate(time.Microsecond)
	secondFetch := firstFetch.Add(1 * time.Hour)

	insertSnapshot := `
		INSERT INTO posting_snapshots (job_posting_id, fetched_at, title, raw_data)
		VALUES ($1, $2, $3, $4::jsonb)
	`
	if _, err := pool.Exec(ctx, insertSnapshot, jobPostingID, firstFetch, "Senior Designer", `{"v":1}`); err != nil {
		t.Fatalf("insert first snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx, insertSnapshot, jobPostingID, secondFetch, "Senior Designer", `{"v":2}`); err != nil {
		t.Fatalf("insert second snapshot: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM posting_snapshots WHERE job_posting_id = $1
	`, jobPostingID).Scan(&count); err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 snapshots for job_posting_id=%d, got %d", jobPostingID, count)
	}

	rows, err := pool.Query(ctx, `
		SELECT fetched_at FROM posting_snapshots
		WHERE job_posting_id = $1
		ORDER BY fetched_at ASC
	`, jobPostingID)
	if err != nil {
		t.Fatalf("query snapshots: %v", err)
	}
	defer rows.Close()

	var fetched []time.Time
	for rows.Next() {
		var ts time.Time
		if err := rows.Scan(&ts); err != nil {
			t.Fatalf("scan fetched_at: %v", err)
		}
		fetched = append(fetched, ts)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration: %v", err)
	}

	if len(fetched) != 2 {
		t.Fatalf("expected 2 fetched_at rows, got %d", len(fetched))
	}
	if !fetched[0].Equal(firstFetch) {
		t.Errorf("first row fetched_at = %v, want %v", fetched[0], firstFetch)
	}
	if !fetched[1].Equal(secondFetch) {
		t.Errorf("second row fetched_at = %v, want %v", fetched[1], secondFetch)
	}
}
