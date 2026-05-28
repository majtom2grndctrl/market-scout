//go:build integration

// Integration tests for internal/db. Require a live Postgres instance and
// DATABASE_URL env var. Run with: go test -tags=integration ./internal/db/...
package db_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/internal/db"
)

// Pins ListLatestDescriptionsByCompany's "latest snapshot per posting, skip
// NULL description" contract end to end. The query uses DISTINCT ON
// (job_posting_id) ORDER BY fetched_at DESC, with a ::text cast forcing a
// non-null Go string. Future readers who change the SQL or sqlc config must
// keep this passing.
func TestListLatestDescriptionsByCompany_SkipsLatestNullDescription(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx := t.Context()

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	queries := db.New(conn)

	// Unique board_token per run avoids collisions with seed data and the
	// (ats, board_token) unique constraint.
	suffix := time.Now().UTC().Format("20060102T150405.000000000")
	boardToken := "market-scout-latestdesc-" + suffix

	var companyID int64
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Market Scout LatestDesc Test", "greenhouse", boardToken).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	var posting1ID, posting2ID int64
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url)
		VALUES ($1, 'ats', $2)
		RETURNING id
	`, companyID, "https://example.com/jobs/"+boardToken+"/1").Scan(&posting1ID); err != nil {
		t.Fatalf("insert job_posting 1: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url)
		VALUES ($1, 'ats', $2)
		RETURNING id
	`, companyID, "https://example.com/jobs/"+boardToken+"/2").Scan(&posting2ID); err != nil {
		t.Fatalf("insert job_posting 2: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM posting_snapshots WHERE job_posting_id IN ($1, $2)`, posting1ID, posting2ID); err != nil {
			t.Logf("cleanup posting_snapshots: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM job_postings WHERE id IN ($1, $2)`, posting1ID, posting2ID); err != nil {
			t.Logf("cleanup job_postings: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM companies WHERE id = $1`, companyID); err != nil {
			t.Logf("cleanup companies: %v", err)
		}
	})

	base := time.Now().UTC().Truncate(time.Microsecond)

	insert := func(t *testing.T, postingID int64, fetchedAt time.Time, description sql.NullString) {
		t.Helper()
		if err := queries.InsertPostingSnapshot(ctx, db.InsertPostingSnapshotParams{
			JobPostingID:    postingID,
			FetchedAt:       fetchedAt,
			RawData:         json.RawMessage(`{}`),
			DescriptionText: description,
		}); err != nil {
			t.Fatalf("InsertPostingSnapshot postingID=%d fetchedAt=%v: %v", postingID, fetchedAt, err)
		}
	}

	// posting 1: older non-null, newer non-null (newer wins).
	insert(t, posting1ID, base.Add(0*time.Hour), sql.NullString{String: "p1 older description", Valid: true})
	insert(t, posting1ID, base.Add(1*time.Hour), sql.NullString{String: "p1 newer description", Valid: true})

	// posting 2: older non-null, newer NULL (latest is NULL → skipped).
	insert(t, posting2ID, base.Add(0*time.Hour), sql.NullString{String: "p2 older description", Valid: true})
	insert(t, posting2ID, base.Add(1*time.Hour), sql.NullString{Valid: false})

	rows, err := queries.ListLatestDescriptionsByCompany(ctx, companyID)
	if err != nil {
		t.Fatalf("ListLatestDescriptionsByCompany: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row (posting 2's latest is NULL and should be skipped), got %d: %#v", len(rows), rows)
	}
	got := rows[0]
	if got.JobPostingID != posting1ID {
		t.Errorf("JobPostingID = %d, want %d", got.JobPostingID, posting1ID)
	}
	if got.DescriptionText != "p1 newer description" {
		t.Errorf("DescriptionText = %q, want %q (the newer snapshot)", got.DescriptionText, "p1 newer description")
	}
}
