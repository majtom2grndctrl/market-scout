//go:build integration

package db_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

func TestFindCompaniesByNormalizedNames_NormalizationOrdinalityDuplicatesAndRecency(t *testing.T) {
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
	baseName := "MS Dedup " + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)
	staleSnapshotAt := now.Add(-45 * 24 * time.Hour)
	recentSnapshotAt := now.Add(-24 * time.Hour)

	seeded := []struct {
		name       string
		ats        string
		token      string
		industry   string
		careersURL string
		snapshotAt *time.Time
	}{
		{
			name:       baseName + ", Inc.",
			ats:        "greenhouse",
			token:      "dedup-normalized-" + suffix + "-a",
			industry:   "robotics",
			careersURL: "https://example.com/a/careers",
			snapshotAt: &recentSnapshotAt,
		},
		{
			name:       "M.S. Dedup " + suffix + " Inc",
			ats:        "ashby",
			token:      "dedup-normalized-" + suffix + "-b",
			industry:   "biotech",
			careersURL: "https://example.com/b/careers",
			snapshotAt: &staleSnapshotAt,
		},
		{
			name:       "Other Dedup " + suffix + " LLC",
			ats:        "lever",
			token:      "dedup-normalized-" + suffix + "-c",
			industry:   "fintech",
			careersURL: "https://example.com/c/careers",
		},
	}

	companyIDs := make([]int64, len(seeded))
	for i, company := range seeded {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO companies (name, ats, board_token, industry, careers_page_url)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, company.name, company.ats, company.token, company.industry, company.careersURL).Scan(&companyIDs[i]); err != nil {
			t.Fatalf("insert company %d: %v", i, err)
		}

		if company.snapshotAt == nil {
			continue
		}

		var jobPostingID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO job_postings (company_id, source_type, source_url)
			VALUES ($1, $2, $3)
			RETURNING id
		`, companyIDs[i], "ats", fmt.Sprintf("https://example.com/jobs/%s", company.token)).Scan(&jobPostingID); err != nil {
			t.Fatalf("insert job_posting %d: %v", i, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO posting_snapshots (job_posting_id, fetched_at, title, raw_data)
			VALUES ($1, $2, $3, '{}'::jsonb)
		`, jobPostingID, *company.snapshotAt, "Designer"); err != nil {
			t.Fatalf("insert posting_snapshot %d: %v", i, err)
		}
	}

	rows, err := db.New(tx).FindCompaniesByNormalizedNames(ctx, db.FindCompaniesByNormalizedNamesParams{
		RecencyDays: 30,
		CandidateNames: []string{
			"M.S. Dedup " + suffix + " Inc",
			baseName,
			"Other-Dedup " + suffix + " LLC",
		},
	})
	if err != nil {
		t.Fatalf("FindCompaniesByNormalizedNames: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3; rows=%+v", len(rows), rows)
	}

	want := []struct {
		inputIndex        int32
		companyID         int64
		name              string
		hasRecentSnapshot bool
		industry          string
		careersURL        string
	}{
		{1, companyIDs[0], seeded[0].name, true, seeded[0].industry, seeded[0].careersURL},
		{1, companyIDs[1], seeded[1].name, false, seeded[1].industry, seeded[1].careersURL},
		{3, companyIDs[2], seeded[2].name, false, seeded[2].industry, seeded[2].careersURL},
	}

	for i, wantRow := range want {
		got := rows[i]
		if got.InputIndex != wantRow.inputIndex {
			t.Fatalf("rows[%d].InputIndex = %d, want %d", i, got.InputIndex, wantRow.inputIndex)
		}
		if got.CompanyID != wantRow.companyID {
			t.Fatalf("rows[%d].CompanyID = %d, want %d", i, got.CompanyID, wantRow.companyID)
		}
		if got.Name != wantRow.name {
			t.Fatalf("rows[%d].Name = %q, want %q", i, got.Name, wantRow.name)
		}
		if got.HasRecentSnapshot != wantRow.hasRecentSnapshot {
			t.Fatalf("rows[%d].HasRecentSnapshot = %v, want %v", i, got.HasRecentSnapshot, wantRow.hasRecentSnapshot)
		}
		if !got.Industry.Valid || got.Industry.String != wantRow.industry {
			t.Fatalf("rows[%d].Industry = %+v, want %q", i, got.Industry, wantRow.industry)
		}
		if !got.CareersPageUrl.Valid || got.CareersPageUrl.String != wantRow.careersURL {
			t.Fatalf("rows[%d].CareersPageUrl = %+v, want %q", i, got.CareersPageUrl, wantRow.careersURL)
		}
	}
}
