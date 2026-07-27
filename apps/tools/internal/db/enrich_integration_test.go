//go:build integration

// Integration tests for apps/tools/internal/db. Require a live Postgres
// instance and DATABASE_URL env var.
// Run with (from apps/tools/): go test -tags=integration ./internal/db/...
package db_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// Pins ListUnclassifiedPostings' @newest_first ORDER BY contract end to end:
// the CASE-based expression must produce a plain ASC sort on first_seen_at
// when NewestFirst is false (the pre-existing default every non-preview
// caller relies on) and a plain DESC sort when NewestFirst is true. Future
// readers who touch the ORDER BY in internal/db/queries/enrich.sql must keep
// this passing.
func TestListUnclassifiedPostings_NewestFirstControlsOrder(t *testing.T) {
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

	suffix := time.Now().UTC().Format("20060102T150405.000000000")
	boardToken := "market-scout-sortorder-" + suffix

	var companyID int64
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Market Scout SortOrder Test", "greenhouse", boardToken).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	base := time.Now().UTC().Truncate(time.Microsecond)
	older := base.Add(-2 * time.Hour)
	newer := base.Add(-1 * time.Hour)

	var olderID, newerID int64
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url, first_seen_at)
		VALUES ($1, 'ats', $2, $3)
		RETURNING id
	`, companyID, "https://example.com/jobs/"+boardToken+"/older", older).Scan(&olderID); err != nil {
		t.Fatalf("insert older job_posting: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url, first_seen_at)
		VALUES ($1, 'ats', $2, $3)
		RETURNING id
	`, companyID, "https://example.com/jobs/"+boardToken+"/newer", newer).Scan(&newerID); err != nil {
		t.Fatalf("insert newer job_posting: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM posting_snapshots WHERE job_posting_id IN ($1, $2)`, olderID, newerID); err != nil {
			t.Logf("cleanup posting_snapshots: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM job_postings WHERE id IN ($1, $2)`, olderID, newerID); err != nil {
			t.Logf("cleanup job_postings: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM companies WHERE id = $1`, companyID); err != nil {
			t.Logf("cleanup companies: %v", err)
		}
	})

	insertSnapshot := func(t *testing.T, postingID int64) {
		t.Helper()
		if err := queries.InsertPostingSnapshot(ctx, db.InsertPostingSnapshotParams{
			JobPostingID:    postingID,
			FetchedAt:       base,
			RawData:         json.RawMessage(`{}`),
			Title:           sql.NullString{String: "Engineer", Valid: true},
			DescriptionText: sql.NullString{String: "sort order test description", Valid: true},
		}); err != nil {
			t.Fatalf("InsertPostingSnapshot postingID=%d: %v", postingID, err)
		}
	}
	insertSnapshot(t, olderID)
	insertSnapshot(t, newerID)

	focus := "sort order test description"

	t.Run("NewestFirst=false orders oldest first", func(t *testing.T) {
		rows, err := queries.ListUnclassifiedPostings(ctx, db.ListUnclassifiedPostingsParams{
			Focus:       focus,
			NewestFirst: false,
			RowLimit:    10,
		})
		if err != nil {
			t.Fatalf("ListUnclassifiedPostings: %v", err)
		}
		ids := postingIDsAmong(rows, olderID, newerID)
		if len(ids) != 2 || ids[0] != olderID || ids[1] != newerID {
			t.Fatalf("posting order = %v, want [older=%d newer=%d]", ids, olderID, newerID)
		}
	})

	t.Run("NewestFirst=true orders newest first", func(t *testing.T) {
		rows, err := queries.ListUnclassifiedPostings(ctx, db.ListUnclassifiedPostingsParams{
			Focus:       focus,
			NewestFirst: true,
			RowLimit:    10,
		})
		if err != nil {
			t.Fatalf("ListUnclassifiedPostings: %v", err)
		}
		ids := postingIDsAmong(rows, olderID, newerID)
		if len(ids) != 2 || ids[0] != newerID || ids[1] != olderID {
			t.Fatalf("posting order = %v, want [newer=%d older=%d]", ids, newerID, olderID)
		}
	})
}

// postingIDsAmong filters rows down to the two posting IDs under test and
// returns them in result order, so the assertion is unaffected by unrelated
// backlog rows a shared dev database might contain.
func postingIDsAmong(rows []db.ListUnclassifiedPostingsRow, wantIDs ...int64) []int64 {
	want := make(map[int64]bool, len(wantIDs))
	for _, id := range wantIDs {
		want[id] = true
	}
	var got []int64
	for _, r := range rows {
		if want[r.PostingID] {
			got = append(got, r.PostingID)
		}
	}
	return got
}
