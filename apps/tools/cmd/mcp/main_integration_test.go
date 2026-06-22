//go:build integration

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
)

func openReadOnlyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_RO")
	if dsn == "" {
		t.Skip("DATABASE_URL_RO not set; skipping integration test")
	}

	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

func TestExecuteReadOnlyQuery_SelectReturnsRows(t *testing.T) {
	pool := openReadOnlyTestDB(t)

	result, err := executeReadOnlyQuery(t.Context(), pool, `
		SELECT 42::bigint AS id, 'ok'::text AS label, TIMESTAMPTZ '2026-06-17 12:00:00+00' AS observed_at
	`)
	if err != nil {
		t.Fatalf("executeReadOnlyQuery: %v", err)
	}

	if result.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", result.RowCount)
	}
	if result.Truncated {
		t.Fatalf("Truncated = true, want false")
	}
	row := result.Rows[0]
	if got, ok := row["id"].(int64); !ok || got != 42 {
		t.Fatalf("id = %#v (%T), want int64(42)", row["id"], row["id"])
	}
	if got, ok := row["label"].(string); !ok || got != "ok" {
		t.Fatalf("label = %#v (%T), want string ok", row["label"], row["label"])
	}
	if got := row["observed_at"]; got != "2026-06-17T12:00:00Z" {
		t.Fatalf("observed_at = %#v, want RFC3339 UTC string", got)
	}
}

func TestExecuteReadOnlyQuery_WriteStatementRejected(t *testing.T) {
	pool := openReadOnlyTestDB(t)
	boardToken := fmt.Sprintf("mcp-readonly-test-%d", time.Now().UnixNano())

	before := countCompaniesByBoardToken(t, pool, boardToken)

	_, err := executeReadOnlyQuery(t.Context(), pool, fmt.Sprintf(`
		INSERT INTO companies (name, ats, board_token)
		VALUES ('MCP Readonly Test', 'greenhouse', '%s')
	`, boardToken))
	if err == nil {
		t.Fatalf("executeReadOnlyQuery write error = nil, want rejection")
	}

	after := countCompaniesByBoardToken(t, pool, boardToken)
	if after != before {
		t.Fatalf("company row count for board_token %q changed from %d to %d", boardToken, before, after)
	}
}

func TestExecuteReadOnlyQuery_TempTableCreationRejected(t *testing.T) {
	pool := openReadOnlyTestDB(t)
	suffix := time.Now().UnixNano()

	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "create temp table",
			sql:  fmt.Sprintf("CREATE TEMP TABLE mcp_readonly_temp_%d (id bigint)", suffix),
		},
		{
			name: "select into temp",
			sql:  fmt.Sprintf("SELECT 1::bigint AS id INTO TEMP mcp_readonly_select_into_%d", suffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeReadOnlyQuery(t.Context(), pool, tt.sql)
			if err == nil {
				t.Fatalf("executeReadOnlyQuery temp table error = nil, want rejection")
			}
		})
	}
}

func countCompaniesByBoardToken(t *testing.T, pool *sql.DB, boardToken string) int64 {
	t.Helper()

	result, err := executeReadOnlyQuery(t.Context(), pool, fmt.Sprintf(`
		SELECT count(*)::bigint AS row_count
		FROM companies
		WHERE board_token = '%s'
	`, boardToken))
	if err != nil {
		t.Fatalf("count companies by board_token: %v", err)
	}
	if result.RowCount != 1 {
		t.Fatalf("count query RowCount = %d, want 1", result.RowCount)
	}

	count, ok := result.Rows[0]["row_count"].(int64)
	if !ok {
		t.Fatalf("row_count = %#v (%T), want int64", result.Rows[0]["row_count"], result.Rows[0]["row_count"])
	}
	return count
}

func TestExecuteReadOnlyQuery_StatementTimeout(t *testing.T) {
	pool := openReadOnlyTestDB(t)

	_, err := executeReadOnlyQuery(t.Context(), pool, `SELECT pg_sleep(6)`)
	if err == nil {
		t.Fatalf("executeReadOnlyQuery timeout error = nil, want timeout")
	}

	message := strings.ToLower(err.Error())
	if !errors.Is(err, context.DeadlineExceeded) &&
		!strings.Contains(message, "statement timeout") &&
		!strings.Contains(message, "canceling statement") {
		t.Fatalf("timeout error = %q, want statement timeout or context deadline", err.Error())
	}
}

func TestExecuteReadOnlyQuery_RowCapTruncates(t *testing.T) {
	pool := openReadOnlyTestDB(t)

	result, err := executeReadOnlyQuery(t.Context(), pool, fmt.Sprintf(`
		SELECT generate_series(1, %d) AS n
	`, rowCap+1))
	if err != nil {
		t.Fatalf("executeReadOnlyQuery: %v", err)
	}
	if result.RowCount != rowCap {
		t.Fatalf("RowCount = %d, want %d", result.RowCount, rowCap)
	}
	if !result.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
}
