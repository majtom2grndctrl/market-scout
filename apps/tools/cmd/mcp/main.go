// Command mcp serves read-only market-scout Postgres queries over MCP stdio.
// See: agent-context/lib/developer-guide.md
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
)

const (
	exitOK           = 0
	exitGenericError = 1

	rowCap           = 1000
	statementTimeout = 5 * time.Second
)

type queryEnvelope struct {
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"row_count"`
	Truncated bool             `json:"truncated"`
}

type fetchStatusEnvelope struct {
	Rows      []fetchStatusRow `json:"rows"`
	RowCount  int              `json:"row_count"`
	Truncated bool             `json:"truncated"`
}

type fetchStatusRow struct {
	Company       string  `json:"company"`
	Status        string  `json:"status"`
	StartedAt     string  `json:"started_at"`
	CompletedAt   *string `json:"completed_at"`
	PostingsCount *int32  `json:"postings_count"`
	ErrorMessage  *string `json:"error_message"`
}

type queryRows interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable body of main. It returns the process exit code so main
// owns only process control, matching the repo's command style.
func run(args []string, stdout, stderr io.Writer) int {
	_ = args
	_ = stdout

	_ = godotenv.Load(".env.local") // no-op if absent; prod sets env vars directly

	logger := slog.New(slog.NewTextHandler(stderr, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := os.Getenv("DATABASE_URL_RO")
	if dsn == "" {
		slog.Error("[mcp] DATABASE_URL_RO is not set")
		return exitGenericError
	}

	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error("[mcp] opening database", "error", err)
		return exitGenericError
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		slog.Error("[mcp] pinging database", "error", err)
		return exitGenericError
	}

	s := newMCPServer(pool)
	slog.Info("[mcp] serving stdio")
	stdioServer := server.NewStdioServer(s)
	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("[mcp] serving stdio", "error", err)
		return exitGenericError
	}

	return exitOK
}

func newMCPServer(pool *sql.DB) *server.MCPServer {
	s := server.NewMCPServer(
		"market-scout-postgres",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	queryTool := mcp.NewTool(
		"query",
		mcp.WithDescription("Run a read-only SQL query against market-scout Postgres and return rows as JSON."),
		mcp.WithString("sql", mcp.Required(), mcp.Description("SQL query to run in a read-only transaction.")),
	)
	s.AddTool(queryTool, queryHandler(pool))

	fetchStatusTool := mcp.NewTool(
		"fetch_status",
		mcp.WithDescription("Return the latest fetch run status for each company."),
	)
	s.AddTool(fetchStatusTool, fetchStatusHandler(pool))

	return s
}

func queryHandler(pool *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sqlText, err := req.RequireString("sql")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result, err := executeReadOnlyQuery(ctx, pool, sqlText)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		payload, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding query result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

func fetchStatusHandler(pool *sql.DB) server.ToolHandlerFunc {
	queries := db.New(pool)

	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_ = req

		queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
		defer cancel()

		rows, err := queries.ListLatestFetchRunsByCompany(queryCtx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("fetch status: %v", err)), nil
		}

		result := fetchStatusEnvelope{
			Rows:      make([]fetchStatusRow, 0, len(rows)),
			Truncated: false,
		}
		for _, row := range rows {
			result.Rows = append(result.Rows, mapFetchStatusRow(row))
		}
		result.RowCount = len(result.Rows)

		payload, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding fetch status: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

func mapFetchStatusRow(row db.ListLatestFetchRunsByCompanyRow) fetchStatusRow {
	return fetchStatusRow{
		Company:       row.Name,
		Status:        row.Status,
		StartedAt:     row.StartedAt.UTC().Format(time.RFC3339),
		CompletedAt:   nullableTimeString(row.CompletedAt),
		PostingsCount: nullableInt32(row.PostingsCount),
		ErrorMessage:  nullableString(row.ErrorMessage),
	}
}

func executeReadOnlyQuery(ctx context.Context, pool *sql.DB, sqlText string) (result queryEnvelope, err error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	conn, err := pool.Conn(queryCtx)
	if err != nil {
		return queryEnvelope{}, fmt.Errorf("open dedicated query connection: %w", err)
	}

	var tx *sql.Tx
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
		if cleanupErr := resetAndCloseConn(conn); cleanupErr != nil && err == nil {
			err = cleanupErr
		}
	}()

	// This gateway intentionally accepts ad-hoc SQL so agents can inspect local
	// data; the read-only role, transaction, and timeouts are its safety rails.
	tx, err = conn.BeginTx(queryCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return queryEnvelope{}, fmt.Errorf("begin read-only transaction: %w", err)
	}

	if _, err := tx.ExecContext(queryCtx, statementTimeoutSQL()); err != nil {
		return queryEnvelope{}, fmt.Errorf("set statement timeout: %w", err)
	}

	rows, err := tx.QueryContext(queryCtx, sqlText)
	if err != nil {
		return queryEnvelope{}, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	result, err = scanQueryRowsWithContext(queryCtx, rows, rowCap)
	if err != nil {
		return queryEnvelope{}, err
	}

	return result, nil
}

func resetAndCloseConn(conn *sql.Conn) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), statementTimeout)
	defer cancel()

	for _, statement := range []string{
		"RESET ALL",
		"UNLISTEN *",
		"SELECT pg_advisory_unlock_all()",
	} {
		if _, err := conn.ExecContext(cleanupCtx, statement); err != nil {
			_ = conn.Raw(func(any) error {
				return driver.ErrBadConn
			})
			if closeErr := conn.Close(); closeErr != nil {
				return fmt.Errorf("reset database session: %w; close tainted connection: %v", err, closeErr)
			}
			return fmt.Errorf("reset database session: %w", err)
		}
	}

	if err := conn.Close(); err != nil {
		return fmt.Errorf("close dedicated query connection: %w", err)
	}
	return nil
}

func scanQueryRows(rows queryRows, cap int) (queryEnvelope, error) {
	return scanQueryRowsWithContext(context.Background(), rows, cap)
}

func scanQueryRowsWithContext(ctx context.Context, rows queryRows, cap int) (queryEnvelope, error) {
	cols, err := rows.Columns()
	if err != nil {
		return queryEnvelope{}, fmt.Errorf("read query columns: %w", err)
	}

	result := queryEnvelope{
		Rows: make([]map[string]any, 0),
	}

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return queryEnvelope{}, fmt.Errorf("read query rows: %w", err)
		}

		if len(result.Rows) >= cap {
			result.Truncated = true
			break
		}

		values := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range values {
			dest[i] = &values[i]
		}

		if err := rows.Scan(dest...); err != nil {
			return queryEnvelope{}, fmt.Errorf("scan query row: %w", err)
		}

		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = normalizeValue(values[i])
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return queryEnvelope{}, fmt.Errorf("read query rows: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return queryEnvelope{}, fmt.Errorf("read query rows: %w", err)
	}

	result.RowCount = len(result.Rows)
	return result, nil
}

func statementTimeoutSQL() string {
	return fmt.Sprintf("SET LOCAL statement_timeout = '%s'", statementTimeout)
}

func normalizeValue(v any) any {
	switch value := v.(type) {
	case []byte:
		return normalizeBytes(value)
	case time.Time:
		return value.UTC().Format(time.RFC3339)
	default:
		return value
	}
}

func normalizeBytes(value []byte) any {
	if json.Valid(value) {
		return json.RawMessage(append([]byte(nil), value...))
	}
	return string(value)
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullableInt32(value sql.NullInt32) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}

func nullableTimeString(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339)
	return &formatted
}
