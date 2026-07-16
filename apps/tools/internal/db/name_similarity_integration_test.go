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

func TestFindCompaniesByNameSimilarity_ScoresAndOrdersMatches(t *testing.T) {
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

	suffix := time.Now().UTC().Format("20060102T150405000000000")
	candidateName := "MS Fuzzy " + suffix + " Robotics"
	companyNames := []string{
		candidateName,
		"MS Fuzzy " + suffix + " Robotics Labs",
	}
	companyIDs := make([]int64, len(companyNames))
	for i, name := range companyNames {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO companies (name, ats, board_token)
			VALUES ($1, $2, $3)
			RETURNING id
		`, name, "greenhouse", fmt.Sprintf("dedup-fuzzy-%s-%d", suffix, i)).Scan(&companyIDs[i]); err != nil {
			t.Fatalf("insert company %d: %v", i, err)
		}
	}

	rows, err := db.New(tx).FindCompaniesByNameSimilarity(ctx, db.FindCompaniesByNameSimilarityParams{
		RecencyDays:         30,
		SimilarityThreshold: 0.4,
		CandidateNames:      []string{candidateName},
	})
	if err != nil {
		t.Fatalf("FindCompaniesByNameSimilarity: %v", err)
	}

	positions := make(map[int64]int, len(companyIDs))
	scores := make(map[int64]float64, len(companyIDs))
	for i, row := range rows {
		if row.InputIndex != 1 {
			t.Fatalf("rows[%d].InputIndex = %d, want 1", i, row.InputIndex)
		}
		if row.Score < 0.4 {
			t.Fatalf("rows[%d].Score = %f, want >= 0.4", i, row.Score)
		}
		if i > 0 && rows[i-1].Score < row.Score {
			t.Fatalf("scores are not descending: rows[%d]=%f, rows[%d]=%f", i-1, rows[i-1].Score, i, row.Score)
		}
		for _, companyID := range companyIDs {
			if row.CompanyID == companyID {
				positions[companyID] = i
				scores[companyID] = row.Score
			}
		}
	}

	for _, companyID := range companyIDs {
		if _, ok := scores[companyID]; !ok {
			t.Fatalf("company %d missing from rows: %+v", companyID, rows)
		}
	}
	if scores[companyIDs[0]] != 1 {
		t.Fatalf("exact normalized match score = %f, want 1", scores[companyIDs[0]])
	}
	if scores[companyIDs[1]] >= scores[companyIDs[0]] {
		t.Fatalf("variant score = %f, want less than exact score %f", scores[companyIDs[1]], scores[companyIDs[0]])
	}
	if positions[companyIDs[0]] >= positions[companyIDs[1]] {
		t.Fatalf("exact match position = %d, variant position = %d; want exact first", positions[companyIDs[0]], positions[companyIDs[1]])
	}
}
