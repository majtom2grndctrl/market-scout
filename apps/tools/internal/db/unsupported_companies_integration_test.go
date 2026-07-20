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

func TestUnsupportedCompanyLookups_MatchNormalizedNameAndLatestURLHost(t *testing.T) {
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
	host := fmt.Sprintf("unsupported-%s.example.com", suffix)
	now := time.Now().UTC().Truncate(time.Microsecond)
	nameMatch := "MS Unsupported " + suffix + ", Inc."
	hostMatch := "MS Shared Host " + suffix

	for _, company := range []struct {
		name          string
		url           string
		detected      any
		reason        string
		firstSeenAt   time.Time
		lastCheckedAt time.Time
	}{
		{
			name: nameMatch, url: "https://www." + host + "/careers", reason: "unsupported_ats",
			firstSeenAt: now.Add(-48 * time.Hour), lastCheckedAt: now.Add(-2 * time.Hour),
		},
		{
			name: hostMatch, url: "https://" + host + "/jobs", detected: "rippling", reason: "unsupported_ats",
			firstSeenAt: now.Add(-24 * time.Hour), lastCheckedAt: now.Add(-time.Hour),
		},
	} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO unsupported_companies (name, url, detected_platform, reason, first_seen_at, last_checked_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, company.name, company.url, company.detected, company.reason, company.firstSeenAt, company.lastCheckedAt); err != nil {
			t.Fatalf("insert unsupported company %q: %v", company.name, err)
		}
	}

	queries := db.New(tx)
	nameRows, err := queries.FindUnsupportedByNames(ctx, []string{"MS.Unsupported " + suffix + " Inc", "No Registry Match"})
	if err != nil {
		t.Fatalf("FindUnsupportedByNames: %v", err)
	}
	if len(nameRows) != 1 {
		t.Fatalf("FindUnsupportedByNames rows = %+v, want one", nameRows)
	}
	if row := nameRows[0]; row.InputIndex != 1 || row.Name != nameMatch || !row.Url.Valid || row.Url.String != "https://www."+host+"/careers" || row.Reason != "unsupported_ats" {
		t.Fatalf("name lookup row = %+v, want normalized name match", row)
	}

	hostRows, err := queries.FindUnsupportedByURLHost(ctx, []string{"https://www." + host + ":8443/openings", "https://unmatched.example/jobs"})
	if err != nil {
		t.Fatalf("FindUnsupportedByURLHost: %v", err)
	}
	if len(hostRows) != 1 {
		t.Fatalf("FindUnsupportedByURLHost rows = %+v, want one", hostRows)
	}
	if row := hostRows[0]; row.InputIndex != 1 || row.Name != hostMatch || !row.DetectedPlatform.Valid || row.DetectedPlatform.String != "rippling" || !row.LastCheckedAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("host lookup row = %+v, want latest shared-host record", row)
	}
}
