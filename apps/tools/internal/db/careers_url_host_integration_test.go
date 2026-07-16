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

func TestFindCompaniesByCareersURLHost_MatchesNormalizedHostname(t *testing.T) {
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
	host := fmt.Sprintf("dedup-%s.example.com", suffix)
	var companyID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token, careers_page_url)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, "MS Domain "+suffix, "greenhouse", "dedup-domain-"+suffix, "https://WWW."+host+":8443/careers?team=eng#openings").Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	candidateURLs := []string{
		"http://" + host + "/jobs",
		"https://WWW." + host + "/jobs",
		"https://" + host + "?team=eng",
		"https://" + host + "#openings",
		"https://" + host + ":9443/jobs",
		"https://unmatched.example/jobs",
	}
	rows, err := db.New(tx).FindCompaniesByCareersURLHost(ctx, db.FindCompaniesByCareersURLHostParams{
		RecencyDays:   30,
		CandidateUrls: candidateURLs,
	})
	if err != nil {
		t.Fatalf("FindCompaniesByCareersURLHost: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5; rows=%+v", len(rows), rows)
	}
	for i, row := range rows {
		wantInputIndex := int32(i + 1)
		if row.InputIndex != wantInputIndex || row.CompanyID != companyID {
			t.Fatalf("rows[%d] = %+v, want input_index=%d company_id=%d", i, row, wantInputIndex, companyID)
		}
		if row.HasRecentSnapshot {
			t.Fatalf("rows[%d].HasRecentSnapshot = true, want false", i)
		}
	}
}
