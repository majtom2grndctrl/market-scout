//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
	"github.com/lib/pq"

	"os"

	"github.com/majtom2grndctrl/market-scout/internal/db"
)

// why: posting_snapshots.location_texts carries a three-valued contract that
// downstream queries depend on:
//
//	nil              -> SQL NULL  ("adapter did not supply a list")
//	[]string{}       -> '{}'      ("adapter ran, source returned zero locations")
//	[]string{...}    -> array     (the data)
//
// The encoder is sqlc-generated: fetcher.sql.go wraps the parameter in
// pq.Array(arg.LocationTexts). pq.Array(nil) writes NULL; pq.Array of an empty
// slice writes '{}'. That distinction is invisible to the unit test in
// cmd/fetcher (which only inspects the params struct) and would break
// silently if sqlc, the driver, or pq.Array ever changed encoding.
//
// This test pins the round trip end to end: insert through the generated
// query, read the row back, assert each branch survives Postgres unchanged.
// A future reader who edits the adapter, the sqlc config, or swaps drivers
// must keep this passing.
func TestSnapshot_LocationTextsNullVsEmptyVsPopulated(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx := t.Context()

	// Production wiring: sqlc was generated for database/sql; the pgx stdlib
	// driver registers under the "pgx" name. Same setup as cmd/fetcher.
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	queries := db.New(conn)

	// Unique board_token per run so the test is robust against leftover state
	// and the (ats, board_token) unique constraint.
	suffix := time.Now().UTC().Format("20060102T150405.000000000")
	boardToken := "market-scout-loctexts-" + suffix

	var companyID int64
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Market Scout LocTexts Test", "greenhouse", boardToken).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	var jobPostingID int64
	if err := conn.QueryRowContext(ctx, `
		INSERT INTO job_postings (company_id, source_type, source_url)
		VALUES ($1, $2, $3)
		RETURNING id
	`, companyID, "ats", "https://example.com/jobs/"+boardToken).Scan(&jobPostingID); err != nil {
		t.Fatalf("insert job_posting: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM posting_snapshots WHERE job_posting_id = $1`, jobPostingID); err != nil {
			t.Logf("cleanup posting_snapshots: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM job_postings WHERE id = $1`, jobPostingID); err != nil {
			t.Logf("cleanup job_postings: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM companies WHERE id = $1`, companyID); err != nil {
			t.Logf("cleanup companies: %v", err)
		}
	})

	cases := []struct {
		name  string
		input []string
		// assert inspects the round-tripped value plus the raw SQL NULL flag
		// observed at the column. The flag is what distinguishes nil from '{}'
		// once the value lands in Go — both surface as a slice via pq.Array,
		// only the column-level NULL bit tells them apart on read.
		assert func(t *testing.T, got []string, sqlIsNull bool)
	}{
		{
			name:  "nil_persists_as_sql_null",
			input: nil,
			assert: func(t *testing.T, got []string, sqlIsNull bool) {
				if !sqlIsNull {
					t.Errorf("expected column to be SQL NULL, got non-null with value %#v", got)
				}
			},
		},
		{
			name:  "empty_slice_persists_as_empty_array",
			input: []string{},
			assert: func(t *testing.T, got []string, sqlIsNull bool) {
				if sqlIsNull {
					t.Errorf("expected non-NULL '{}' array, got SQL NULL")
				}
				if got == nil {
					t.Errorf("expected non-nil empty slice, got nil")
				}
				if len(got) != 0 {
					t.Errorf("expected length 0, got %d (%#v)", len(got), got)
				}
			},
		},
		{
			name:  "populated_round_trips_intact",
			input: []string{"NYC", "SF"},
			assert: func(t *testing.T, got []string, sqlIsNull bool) {
				if sqlIsNull {
					t.Errorf("expected non-NULL array, got SQL NULL")
				}
				want := []string{"NYC", "SF"}
				if len(got) != len(want) {
					t.Fatalf("length mismatch: got %#v, want %#v", got, want)
				}
				for i := range want {
					if got[i] != want[i] {
						t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
					}
				}
			},
		},
	}

	// Each case gets its own fetched_at so rows are unambiguous to read back.
	base := time.Now().UTC().Truncate(time.Microsecond)

	for i, tc := range cases {
		tc := tc
		fetchedAt := base.Add(time.Duration(i) * time.Minute)

		t.Run(tc.name, func(t *testing.T) {
			err := queries.InsertPostingSnapshot(ctx, db.InsertPostingSnapshotParams{
				JobPostingID:  jobPostingID,
				FetchedAt:     fetchedAt,
				RawData:       json.RawMessage(`{}`),
				LocationTexts: tc.input,
			})
			if err != nil {
				t.Fatalf("InsertPostingSnapshot: %v", err)
			}

			// Read back via raw SQL so we can observe the column-level NULL
			// flag (the generated row type would hide that distinction). The
			// IS NULL projection is the only place the nil-vs-empty contract
			// is visible on the read side.
			var got []string
			var isNull bool
			row := conn.QueryRowContext(ctx, `
				SELECT location_texts, location_texts IS NULL
				FROM posting_snapshots
				WHERE job_posting_id = $1 AND fetched_at = $2
			`, jobPostingID, fetchedAt)
			if err := row.Scan(pq.Array(&got), &isNull); err != nil {
				t.Fatalf("scan: %v", err)
			}

			tc.assert(t, got, isNull)
		})
	}
}
