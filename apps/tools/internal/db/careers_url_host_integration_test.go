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
	`, "MS Domain "+suffix, "greenhouse", "dedup-domain-"+suffix, "https://user:pass@WWW."+host+":8443/careers?team=eng#openings").Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}
	var ipv6CompanyID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO companies (name, ats, board_token, careers_page_url)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, "MS IPv6 "+suffix, "greenhouse", "dedup-ipv6-"+suffix, "https://user@[2001:DB8::1]:8443/careers").Scan(&ipv6CompanyID); err != nil {
		t.Fatalf("insert IPv6 company: %v", err)
	}

	candidateURLs := []string{
		"http://" + host + "/jobs",
		"https://WWW." + host + "/jobs",
		"https://" + host + "?team=eng",
		"https://" + host + "#openings",
		"https://" + host + ":9443/jobs",
		"https://candidate:secret@" + host + "/jobs",
		"https://unmatched.example/jobs",
		"http://[2001:db8::1]/jobs",
		"https://candidate@[2001:DB8::1]:9443/jobs?team=eng#openings",
	}
	rows, err := db.New(tx).FindCompaniesByCareersURLHost(ctx, db.FindCompaniesByCareersURLHostParams{
		RecencyDays:   30,
		CandidateUrls: candidateURLs,
	})
	if err != nil {
		t.Fatalf("FindCompaniesByCareersURLHost: %v", err)
	}
	if len(rows) != 8 {
		t.Fatalf("len(rows) = %d, want 8; rows=%+v", len(rows), rows)
	}
	want := []struct {
		inputIndex int32
		companyID  int64
	}{
		{1, companyID},
		{2, companyID},
		{3, companyID},
		{4, companyID},
		{5, companyID},
		{6, companyID},
		{8, ipv6CompanyID},
		{9, ipv6CompanyID},
	}
	for i, row := range rows {
		if row.InputIndex != want[i].inputIndex || row.CompanyID != want[i].companyID {
			t.Fatalf("rows[%d] = %+v, want input_index=%d company_id=%d", i, row, want[i].inputIndex, want[i].companyID)
		}
		if row.HasRecentSnapshot {
			t.Fatalf("rows[%d].HasRecentSnapshot = true, want false", i)
		}
	}
}
