//go:build integration

package db_test

import (
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
)

// why: posting_requisitions is the read model's second denominator. Its
// contract is not "the SQL runs" but four properties a consumer counts on:
// exactly one row per open posting, requisition_key never NULL,
// requisition_source telling a platform-supplied key apart from a synthesized
// fallback, and company_id traveling with the key so consumers count distinct
// over (company_id, requisition_key) rather than the bare key. None of the
// four is visible from the view definition alone -- the first three need a
// company that mixes snapshots carrying a key with snapshots that do not, and
// the fourth needs two companies that reuse the same requisition key, which is
// exactly the board-scoped-key collision 000028_posting_requisitions_view
// documents (two Workable boards numbering by year-week collide on their first
// overlap).
//
// Every fixture row lives inside a transaction that is rolled back. This suite
// reads DATABASE_URL, which is the development database, and a leaked fixture
// company with a NOT NULL ats is exactly what the fetch list selects on -- it
// would become a permanent fetcher target.
func TestPostingRequisitions_OneRowPerOpenPostingWithSourceLabel(t *testing.T) {
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

	suffix := time.Now().UTC().Format("20060102t150405000000000")

	var companyID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "MS Requisitions "+suffix, "greenhouse", "ms-requisitions-"+suffix).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	runStart := time.Now().UTC().Truncate(time.Microsecond)

	// A successful run establishes openness. The failed run below writes a
	// later snapshot for the same posting; the view's run-scoped lateral must
	// ignore it, so its requisition_key is a value no assertion expects.
	var successRunID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO fetch_runs (company_id, started_at, status)
		VALUES ($1, $2, 'success')
		RETURNING id
	`, companyID, runStart).Scan(&successRunID); err != nil {
		t.Fatalf("insert success fetch_run: %v", err)
	}
	var failedRunID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO fetch_runs (company_id, started_at, status)
		VALUES ($1, $2, 'failed')
		RETURNING id
	`, companyID, runStart.Add(time.Hour)).Scan(&failedRunID); err != nil {
		t.Fatalf("insert failed fetch_run: %v", err)
	}

	// Two postings share requisition key REQ-1: the shape this whole plan
	// exists for, one requisition listed once per location. A third carries its
	// own key, and a fourth carries none and must fall back.
	fixtures := []struct {
		slug           string
		requisitionKey any
	}{
		{"loc-a", "REQ-1"},
		{"loc-b", "REQ-1"},
		{"solo", "REQ-2"},
		{"keyless", nil},
	}

	postingIDs := make(map[string]int64, len(fixtures))
	for _, f := range fixtures {
		var postingID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO job_postings (company_id, source_type, source_url)
			VALUES ($1, 'ats', $2)
			RETURNING id
		`, companyID, "https://example.com/jobs/"+suffix+"/"+f.slug).Scan(&postingID); err != nil {
			t.Fatalf("insert job_posting %s: %v", f.slug, err)
		}
		postingIDs[f.slug] = postingID

		// Two snapshots in the successful run: the later one is current.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO posting_snapshots (job_posting_id, fetch_run_id, fetched_at, title, raw_data, requisition_key)
			VALUES ($1, $2, $3, $4, '{}'::jsonb, $5)
		`, postingID, successRunID, runStart, "Stale "+f.slug, "STALE"); err != nil {
			t.Fatalf("insert stale snapshot %s: %v", f.slug, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO posting_snapshots (job_posting_id, fetch_run_id, fetched_at, title, raw_data, requisition_key)
			VALUES ($1, $2, $3, $4, '{}'::jsonb, $5)
		`, postingID, successRunID, runStart.Add(time.Minute), "Current "+f.slug, f.requisitionKey); err != nil {
			t.Fatalf("insert current snapshot %s: %v", f.slug, err)
		}
		// Written by the failed run, later than everything above.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO posting_snapshots (job_posting_id, fetch_run_id, fetched_at, title, raw_data, requisition_key)
			VALUES ($1, $2, $3, $4, '{}'::jsonb, $5)
		`, postingID, failedRunID, runStart.Add(2*time.Hour), "Failed run "+f.slug, "FAILED-RUN"); err != nil {
			t.Fatalf("insert failed-run snapshot %s: %v", f.slug, err)
		}
	}

	type row struct {
		companyID int64
		key       string
		source    string
	}
	got := make(map[int64]row, len(fixtures))

	rows, err := tx.QueryContext(ctx, `
		SELECT job_posting_id, company_id, requisition_key, requisition_source
		FROM posting_requisitions
		WHERE company_id = $1
	`, companyID)
	if err != nil {
		t.Fatalf("query posting_requisitions: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postingID int64
		var r row
		var key sql.NullString
		if err := rows.Scan(&postingID, &r.companyID, &key, &r.source); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if !key.Valid {
			t.Errorf("job_posting_id %d: requisition_key is NULL; the view must always supply a key", postingID)
		}
		r.key = key.String
		if _, dup := got[postingID]; dup {
			t.Errorf("job_posting_id %d appeared more than once; the view must emit one row per open posting", postingID)
		}
		got[postingID] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rows: %v", err)
	}

	if len(got) != len(fixtures) {
		t.Fatalf("got %d rows for the fixture company, want %d (one per open posting)", len(got), len(fixtures))
	}

	keylessKey := "posting:" + strconv.FormatInt(postingIDs["keyless"], 10)
	want := map[string]row{
		"loc-a":   {companyID: companyID, key: "REQ-1", source: "ats"},
		"loc-b":   {companyID: companyID, key: "REQ-1", source: "ats"},
		"solo":    {companyID: companyID, key: "REQ-2", source: "ats"},
		"keyless": {companyID: companyID, key: keylessKey, source: "posting"},
	}
	for slug, wantRow := range want {
		t.Run(slug, func(t *testing.T) {
			gotRow, ok := got[postingIDs[slug]]
			if !ok {
				t.Fatalf("no row for posting %s (id %d)", slug, postingIDs[slug])
			}
			if gotRow != wantRow {
				t.Errorf("posting %s: got %+v, want %+v", slug, gotRow, wantRow)
			}
		})
	}

	// The denominator this view exists to produce: four open postings, three
	// distinct requisitions once the duplicated listing collapses.
	var postings, requisitions int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*), count(DISTINCT (company_id, requisition_key))
		FROM posting_requisitions
		WHERE company_id = $1
	`, companyID).Scan(&postings, &requisitions); err != nil {
		t.Fatalf("count requisitions: %v", err)
	}
	if postings != 4 || requisitions != 3 {
		t.Errorf("got %d postings / %d requisitions, want 4 / 3", postings, requisitions)
	}

	// A second company reusing REQ-1 exercises why company_id travels with the
	// key at all: requisition keys are board-scoped, so two companies that
	// number requisitions the same way collide on the bare key. One open
	// posting is enough -- the other three properties asserted above already
	// come from company A's fixtures.
	var otherCompanyID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "MS Requisitions Other "+suffix, "greenhouse", "ms-requisitions-other-"+suffix).Scan(&otherCompanyID); err != nil {
		t.Fatalf("insert other company: %v", err)
	}
	var otherRunID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO fetch_runs (company_id, started_at, status)
		VALUES ($1, $2, 'success')
		RETURNING id
	`, otherCompanyID, runStart).Scan(&otherRunID); err != nil {
		t.Fatalf("insert other company fetch_run: %v", err)
	}
	var otherPostingID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url)
		VALUES ($1, 'ats', $2)
		RETURNING id
	`, otherCompanyID, "https://example.com/jobs/"+suffix+"/other-req-1").Scan(&otherPostingID); err != nil {
		t.Fatalf("insert other company job_posting: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO posting_snapshots (job_posting_id, fetch_run_id, fetched_at, title, raw_data, requisition_key)
		VALUES ($1, $2, $3, $4, '{}'::jsonb, $5)
	`, otherPostingID, otherRunID, runStart, "Current other-req-1", "REQ-1"); err != nil {
		t.Fatalf("insert other company snapshot: %v", err)
	}

	// Both companies together: 5 open postings (4 from company A, 1 from
	// company B), 4 requisitions once counted per (company_id,
	// requisition_key) -- company B's REQ-1 is distinct from company A's
	// REQ-1. A bare count(DISTINCT requisition_key) merges the two companies'
	// identical keys and undercounts to 3, which is the regression this
	// fixture exists to catch: drop company_id from the view, or from a
	// consumer's count, and this assertion is the only one that fails.
	var totalPostings, scopedRequisitions, bareRequisitions int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			count(*),
			count(DISTINCT (company_id, requisition_key)),
			count(DISTINCT requisition_key)
		FROM posting_requisitions
		WHERE company_id IN ($1, $2)
	`, companyID, otherCompanyID).Scan(&totalPostings, &scopedRequisitions, &bareRequisitions); err != nil {
		t.Fatalf("count requisitions across companies: %v", err)
	}
	if totalPostings != 5 || scopedRequisitions != 4 {
		t.Errorf("got %d postings / %d company-scoped requisitions across two companies, want 5 / 4", totalPostings, scopedRequisitions)
	}
	if bareRequisitions != 3 {
		t.Errorf("got %d for bare count(DISTINCT requisition_key) across two companies, want 3 (REQ-1, REQ-2, and the keyless posting's synthesized key, with company A's and company B's REQ-1 merged)", bareRequisitions)
	}
	if bareRequisitions == scopedRequisitions {
		t.Errorf("bare count(DISTINCT requisition_key) (%d) equals the company-scoped count (%d); they must differ so this test fails if company_id is dropped from the view or from a consumer's count", bareRequisitions, scopedRequisitions)
	}
}
