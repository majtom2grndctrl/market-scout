//go:build integration

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

// Pins ListPostingsByIDs's exact-ID repair contract: rows come back in the
// caller's order, each with its latest snapshot's text, and a posting whose
// latest snapshot has no description is left out, so the caller can tell the
// cohort is partial.
func TestListPostingsByIDs_OrderLatestAndNullDescription(t *testing.T) {
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
	queries := db.New(conn)

	suffix := time.Now().UTC().Format("20060102T150405.000000000")
	boardToken := "market-scout-byids-" + suffix

	var companyID int64
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Market Scout ByIDs Test", "greenhouse", boardToken).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	var ids [3]int64
	for i := range ids {
		if err := conn.QueryRowContext(ctx, `
			INSERT INTO job_postings (company_id, source_type, source_url)
			VALUES ($1, 'ats', $2)
			RETURNING id
		`, companyID, "https://example.com/jobs/"+boardToken+"/"+string(rune('a'+i))).Scan(&ids[i]); err != nil {
			t.Fatalf("insert job_posting %d: %v", i, err)
		}
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, q := range []string{
			`DELETE FROM posting_snapshots WHERE job_posting_id = ANY($1)`,
			`DELETE FROM job_postings WHERE id = ANY($1)`,
		} {
			if _, err := conn.ExecContext(cleanupCtx, q, ids[:]); err != nil {
				t.Logf("cleanup: %v", err)
			}
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM companies WHERE id = $1`, companyID); err != nil {
			t.Logf("cleanup companies: %v", err)
		}
	})

	base := time.Now().UTC().Truncate(time.Microsecond)
	insert := func(postingID int64, fetchedAt time.Time, title, description string) {
		t.Helper()
		if err := queries.InsertPostingSnapshot(ctx, db.InsertPostingSnapshotParams{
			JobPostingID:    postingID,
			FetchedAt:       fetchedAt,
			RawData:         json.RawMessage(`{}`),
			Title:           sql.NullString{String: title, Valid: true},
			DescriptionText: sql.NullString{String: description, Valid: description != ""},
		}); err != nil {
			t.Fatalf("InsertPostingSnapshot postingID=%d: %v", postingID, err)
		}
	}
	insert(ids[0], base, "Old title", "older text")
	insert(ids[0], base.Add(time.Hour), "New title", "newer text")
	insert(ids[1], base, "Two", "two text")
	insert(ids[2], base, "Three", "three text")
	insert(ids[2], base.Add(time.Hour), "Three", "") // latest has no description

	// Requested out of id order: the caller's order wins.
	rows, err := queries.ListPostingsByIDs(ctx, []int64{ids[1], ids[0]})
	if err != nil {
		t.Fatalf("ListPostingsByIDs: %v", err)
	}
	if len(rows) != 2 || rows[0].PostingID != ids[1] || rows[1].PostingID != ids[0] {
		t.Fatalf("rows = %+v, want postings %d then %d", rows, ids[1], ids[0])
	}
	if rows[1].Title.String != "New title" || rows[1].DescriptionText.String != "newer text" {
		t.Errorf("posting %d = %q / %q, want the latest snapshot's title and text", ids[0], rows[1].Title.String, rows[1].DescriptionText.String)
	}

	rows, err = queries.ListPostingsByIDs(ctx, []int64{ids[0], ids[2]})
	if err != nil {
		t.Fatalf("ListPostingsByIDs: %v", err)
	}
	if len(rows) != 1 || rows[0].PostingID != ids[0] {
		t.Fatalf("rows = %+v, want only posting %d: %d's latest snapshot has no description", rows, ids[0], ids[2])
	}
}
