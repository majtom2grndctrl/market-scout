//go:build integration

package db_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// why: the adapters extract a requisition key and cmd/fetcher forwards it, but
// both of those are proven only in unit tests against in-memory structs. The
// column itself was populated by a one-time backfill reading raw_data, so no
// row in either database has ever been written through the generated insert
// with a requisition key on it. That leaves the seam this test covers --
// InsertPostingSnapshotParams.RequisitionKey reaching posting_snapshots and
// surfacing through posting_requisitions -- unexercised end to end. A dropped
// column here compiles clean, passes every existing test, and silently writes
// NULL forever.
//
// Both branches matter and they are not symmetric: a set key must arrive
// verbatim with requisition_source 'ats', and an unset key must produce the
// synthesized 'posting:<id>' fallback rather than a NULL the consumer would
// count through as one requisition for the whole company.
//
// Every fixture row lives inside a transaction that is rolled back. This suite
// reads DATABASE_URL, the development database, and a leaked fixture company
// with a NOT NULL ats is exactly what the fetch list selects on -- it would
// become a permanent fetcher target.
func TestInsertPostingSnapshot_PersistsRequisitionKeyThroughToView(t *testing.T) {
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

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	suffix := time.Now().UTC().Format("20060102T150405.000000000")
	fetchedAt := time.Now().UTC()

	var companyID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, 'greenhouse', $2)
		RETURNING id
	`, "MS WritePath "+suffix, "ms-writepath-"+suffix).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	// open_postings derives openness from the latest successful run, and the
	// view's lateral is run-scoped, so the snapshot must carry this run's id.
	var fetchRunID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO fetch_runs (company_id, started_at, status)
		VALUES ($1, $2, 'success')
		RETURNING id
	`, companyID, fetchedAt.Add(-time.Minute)).Scan(&fetchRunID); err != nil {
		t.Fatalf("insert fetch_run: %v", err)
	}

	queries := db.New(tx)

	cases := []struct {
		name       string
		key        sql.NullString
		wantKey    string // empty means "expect the posting: fallback"
		wantSource string
	}{
		{
			name:       "key_set_arrives_verbatim",
			key:        sql.NullString{String: "REQ-WRITEPATH-1", Valid: true},
			wantKey:    "REQ-WRITEPATH-1",
			wantSource: "ats",
		},
		{
			name:       "key_unset_falls_back_to_posting_identity",
			key:        sql.NullString{},
			wantSource: "posting",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var jobPostingID int64
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO job_postings (company_id, source_type, source_url)
				VALUES ($1, 'ats', $2)
				RETURNING id
			`, companyID, "https://example.com/jobs/"+suffix+"/"+tc.name).Scan(&jobPostingID); err != nil {
				t.Fatalf("insert job_posting: %v", err)
			}

			if err := queries.InsertPostingSnapshot(ctx, db.InsertPostingSnapshotParams{
				JobPostingID:   jobPostingID,
				FetchedAt:      fetchedAt,
				Title:          sql.NullString{String: "Write Path " + tc.name, Valid: true},
				RawData:        json.RawMessage(`{}`),
				LocationTexts:  []string{"Seattle, WA"},
				FetchRunID:     sql.NullInt64{Int64: fetchRunID, Valid: true},
				RequisitionKey: tc.key,
			}); err != nil {
				t.Fatalf("InsertPostingSnapshot: %v", err)
			}

			// Read the stored column first: if the generated insert dropped the
			// field, the view's COALESCE would hide it behind the fallback and
			// the source label alone could not tell the two failures apart.
			var stored sql.NullString
			if err := tx.QueryRowContext(ctx, `
				SELECT requisition_key FROM posting_snapshots WHERE job_posting_id = $1
			`, jobPostingID).Scan(&stored); err != nil {
				t.Fatalf("read back posting_snapshots.requisition_key: %v", err)
			}
			if stored.Valid != tc.key.Valid || stored.String != tc.key.String {
				t.Errorf("posting_snapshots.requisition_key = %#v, want %#v", stored, tc.key)
			}

			var gotKey, gotSource string
			var gotCompanyID int64
			if err := tx.QueryRowContext(ctx, `
				SELECT requisition_key, requisition_source, company_id
				FROM posting_requisitions WHERE job_posting_id = $1
			`, jobPostingID).Scan(&gotKey, &gotSource, &gotCompanyID); err != nil {
				t.Fatalf("read posting_requisitions: %v", err)
			}

			wantKey := tc.wantKey
			if wantKey == "" {
				wantKey = "posting:" + strconv.FormatInt(jobPostingID, 10)
			}
			if gotKey != wantKey {
				t.Errorf("requisition_key = %q, want %q", gotKey, wantKey)
			}
			if gotSource != tc.wantSource {
				t.Errorf("requisition_source = %q, want %q", gotSource, tc.wantSource)
			}
			if gotCompanyID != companyID {
				t.Errorf("company_id = %d, want %d", gotCompanyID, companyID)
			}
		})
	}
}
