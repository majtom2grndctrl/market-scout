// Command mcp is the market-scout MCP server. Read-only inspection tools bind
// DATABASE_URL_RO; curated write tools bind DATABASE_URL_ACTIONS (action role,
// EXECUTE-only on approved mcp.* SECURITY DEFINER functions).
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

	pools, cleanup, err := openPools(ctx)
	if err != nil {
		// err already names the misconfigured DSN by env var without exposing its value.
		slog.Error("[mcp] opening database pools", "error", err)
		return exitGenericError
	}
	defer cleanup()

	s := newMCPServer(pools)
	slog.Info("[mcp] serving stdio")
	stdioServer := server.NewStdioServer(s)
	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("[mcp] serving stdio", "error", err)
		return exitGenericError
	}

	return exitOK
}

// dbPools holds the two connections the MCP server uses. The read-only pool
// backs the generic query gateway and other inspection tools; the action pool
// is reserved for curated write tools that call approved SECURITY DEFINER
// functions. The action pool is never handed to the read-only query handler.
type dbPools struct {
	readOnly *sql.DB
	action   *sql.DB
}

// openPools opens and verifies both database connections. Both DSNs are
// required: a misconfigured action DSN is a startup failure, not a degraded
// mode, so a leaked broad DSN can never silently substitute for the curated
// action path. Errors name the offending env var but never its value.
func openPools(ctx context.Context) (pools dbPools, cleanup func(), err error) {
	roPool, err := openVerifiedPool(ctx, "DATABASE_URL_RO")
	if err != nil {
		return dbPools{}, nil, err
	}

	actionPool, err := openVerifiedPool(ctx, "DATABASE_URL_ACTIONS")
	if err != nil {
		roPool.Close()
		return dbPools{}, nil, err
	}

	cleanup = func() {
		actionPool.Close()
		roPool.Close()
	}
	return dbPools{readOnly: roPool, action: actionPool}, cleanup, nil
}

// openVerifiedPool reads the DSN from envVar, opens a pool, and pings it. The
// caller owns Close on success. On failure it reports the env var name only;
// the DSN value (which carries credentials) never reaches logs or errors.
func openVerifiedPool(ctx context.Context, envVar string) (*sql.DB, error) {
	dsn := os.Getenv(envVar)
	if dsn == "" {
		return nil, fmt.Errorf("%s is not set", envVar)
	}

	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", envVar, err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		pool.Close()
		// The driver's ping error embeds connection parameters (user, host,
		// database) parsed from the DSN, so it is not wrapped here: leaking
		// credentials in startup logs would defeat the action-boundary design.
		return nil, fmt.Errorf("pinging %s failed (connection refused or unreachable); DSN value omitted from this error", envVar)
	}

	return pool, nil
}

func newMCPServer(pools dbPools) *server.MCPServer {
	s := server.NewMCPServer(
		"market-scout-postgres",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	// Read-only inspection tools always bind to the read-only pool. Curated write
	// tools bind to the action pool (pools.action), which can call only approved
	// mcp.* SECURITY DEFINER functions — never the read-only query gateway.
	queryTool := mcp.NewTool(
		"query",
		mcp.WithDescription("Run a read-only SQL query against market-scout Postgres and return rows as JSON."),
		mcp.WithString("sql", mcp.Required(), mcp.Description("SQL query to run in a read-only transaction.")),
	)
	s.AddTool(queryTool, queryHandler(pools.readOnly))

	fetchStatusTool := mcp.NewTool(
		"fetch_status",
		mcp.WithDescription("Return the latest fetch run status for each company."),
	)
	s.AddTool(fetchStatusTool, fetchStatusHandler(pools.readOnly))

	// enrichment_preview is read-only: it reports what batch-enrich would select
	// without spawning agents, so it binds the read-only pool. The RO role
	// already has the table reads the shared selection core needs.
	enrichmentPreviewTool := mcp.NewTool(
		"enrichment_preview",
		mcp.WithDescription("Preview which job postings batch-enrich would select for classification, without spawning agents or writing anything."),
		mcp.WithNumber("count", mcp.Description("Max postings to select (1-100). Defaults to 10.")),
		mcp.WithString("focus", mcp.Description("ILIKE prefilter on title and description; `%` and `_` are SQL wildcards. Empty means no filter.")),
		mcp.WithBoolean("force", mcp.Description("Include already-classified postings (drops the unclassified filter). Defaults to false.")),
		mcp.WithString("sort", mcp.Description("Selection order by first_seen_at: \"oldest_first\" (default; processes the backlog in stable priority order) or \"newest_first\" (previews a recency-biased selection). Use newest_first when the batch-enrich run you're previewing will select newest postings first.")),
	)
	s.AddTool(enrichmentPreviewTool, enrichmentPreviewHandler(pools.readOnly))

	dedupCandidatesTool := mcp.NewTool(
		"dedup_candidates",
		mcp.WithDescription("Classify discovered company candidates as new, duplicate, stale, or invalid without probing, writing, or opening a browser."),
		mcp.WithArray("candidates",
			mcp.Required(),
			mcp.Description("Candidate companies to classify, in order. Max 200. Each item has required name and optional ats/board_token/careers_url."),
			mcp.MaxItems(dedupMaxCandidates),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        map[string]any{"type": "string", "description": "Company display name."},
					"ats":         map[string]any{"type": "string", "description": "Optional ATS key when already known."},
					"board_token": map[string]any{"type": "string", "description": "Optional ATS board token when already known."},
					"careers_url": map[string]any{"type": "string", "description": "Optional absolute http(s) careers-page URL used for host matching."},
				},
				"required": []string{"name"},
			}),
		),
		mcp.WithInteger("recency_days", mcp.Description("Snapshot recency window in days, applied to the whole batch. Defaults to 30.")),
	)
	s.AddTool(dedupCandidatesTool, dedupCandidatesHandler(pools.readOnly))

	detectATSTool := mcp.NewTool(
		"detect_ats",
		mcp.WithDescription("Parse supplied careers and observed URLs for supported ATS board evidence without probing, querying, or writing."),
		mcp.WithString("careers_url", mcp.Description("Primary absolute http(s) URL evidence, usually the visible careers page or final ATS URL.")),
		mcp.WithArray("observed_urls", mcp.WithStringItems(), mcp.Description("Ordered absolute http(s) URLs observed in redirects, page links, scripts, or network requests.")),
	)
	s.AddTool(detectATSTool, detectATSHandler())

	// add_company is an action tool: it writes through the approved
	// mcp.add_company function, so it binds the action pool, not the read-only
	// pool. The agent sends only typed parameters; the server never relays SQL.
	addCompanyTool := mcp.NewTool(
		"add_company",
		mcp.WithDescription("Validate and insert a company through the approved mcp.add_company action function. Optionally probes the live ATS board first."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Company display name.")),
		mcp.WithString("ats", mcp.Required(), mcp.Description("ATS key: greenhouse, lever, ashby, workday, or workable.")),
		mcp.WithString("board_token", mcp.Required(), mcp.Description("ATS board token. Workday is {host}/{site}; Workable is the account slug.")),
		mcp.WithString("industry", mcp.Description("Optional industry label, stored verbatim.")),
		mcp.WithString("careers_page_url", mcp.Description("Optional absolute http(s) careers page URL.")),
		mcp.WithBoolean("probe", mcp.Description("Probe the live ATS board before inserting. Defaults to true.")),
	)
	s.AddTool(addCompanyTool, addCompanyHandler(pools.action))

	recordUnsupportedCompanyTool := mcp.NewTool(
		"record_unsupported_company",
		mcp.WithDescription("Record informational metadata for a company with an unsupported ATS or no careers page through the approved action function."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Company display name.")),
		mcp.WithString("reason", mcp.Required(), mcp.Description("Registry reason: unsupported_ats or no_careers.")),
		mcp.WithString("url", mcp.Description("Absolute http(s) URL. Required for unsupported_ats and optional for no_careers.")),
		mcp.WithString("detected_platform", mcp.Description("Optional detected unsupported platform label.")),
	)
	s.AddTool(recordUnsupportedCompanyTool, recordUnsupportedCompanyHandler(pools.action))

	// save_enrichment is an action tool: it persists a classification through the
	// approved mcp.save_enrichment function, so the write binds the action pool.
	// It also reads taxonomy and posting existence for pre-validation through the
	// read-only pool — the RO role already has those table reads. The agent sends
	// a classifier-shaped payload plus provenance; the server never relays SQL.
	saveEnrichmentTool := mcp.NewTool(
		"save_enrichment",
		mcp.WithDescription("Persist a classifier-shaped enrichment for a job posting through the approved mcp.save_enrichment action function. Append-only: every call inserts a new classification row and never edits prior history."),
		mcp.WithNumber("posting_id", mcp.Required(), mcp.Description("Target job posting id; must already exist.")),
		mcp.WithObject("provenance", mcp.Description("Optional. model defaults to \"mcp-agent\", prompt_version to \"mcp-save-enrichment-v1\". Both must match ^[A-Za-z0-9._-]+$ after defaults.")),
		mcp.WithObject("classification", mcp.Required(), mcp.Description("seniority (required closed set) and optional notes.")),
		mcp.WithArray("canonical_roles", mcp.Description("Canonical roles: each {slug, name, dimensions[]}. Dimensions are a closed seeded set.")),
		mcp.WithArray("specializations", mcp.Description("Specializations: each {slug, name}.")),
		mcp.WithArray("skills", mcp.Description("Skills: each {slug, name, optional requirement}. requirement is echoed but not persisted.")),
		mcp.WithString("summary", mcp.Description("Echoed in the response only; not persisted.")),
	)
	s.AddTool(saveEnrichmentTool, saveEnrichmentHandler(pools.readOnly, pools.action))

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

func scanQueryRows(rows queryRows, rowCap int) (queryEnvelope, error) {
	return scanQueryRowsWithContext(context.Background(), rows, rowCap)
}

func scanQueryRowsWithContext(ctx context.Context, rows queryRows, rowCap int) (queryEnvelope, error) {
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

		if len(result.Rows) >= rowCap {
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
