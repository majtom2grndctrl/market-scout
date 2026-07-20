//go:build integration

// Integration tests for the MCP action boundary (Task 8 of the MCP Safe Actions
// plan). They exercise the real action role (DATABASE_URL_ACTIONS), the
// read-only role (DATABASE_URL_RO), and the owner DSN (DATABASE_URL) against a
// live Postgres.
//
// The enforceable invariant: a leaked DATABASE_URL_ACTIONS DSN can call ONLY the
// approved mcp.* SECURITY DEFINER functions — never arbitrary table writes, DDL,
// or temp tables. Classifications are append-only: save_enrichment inserts a new
// row every call and never mutates prior history.
//
// Approach per scenario:
//   - #1 add_company, #2/#8 save_enrichment, #7 idempotency: drive the actual Go
//     handlers (add_company / save_enrichment) against the action pool, so the
//     test proves the real path, not just the SQL function.
//   - #3-#6 boundary denials: direct SQL through the role DSNs, since the point is
//     the role's raw privilege set, not a Go handler.
//
// Every test seeds its own rows via DATABASE_URL and removes them in t.Cleanup.
// Row-count assertions are scoped to test-specific data (unique board tokens, a
// seeded posting id) so they never depend on existing DB contents.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
)

// openActionTestPools opens all three pools the action integration tests need.
// It skips (never fails) when ANY required DSN is absent, per the testing-guide
// rule that env-gated tests skip rather than fail without their environment.
func openActionTestPools(t *testing.T) (owner, readOnly, action *sql.DB) {
	t.Helper()

	owner = openTestPoolFromEnv(t, "DATABASE_URL")
	readOnly = openTestPoolFromEnv(t, "DATABASE_URL_RO")
	action = openTestPoolFromEnv(t, "DATABASE_URL_ACTIONS")
	return owner, readOnly, action
}

// openTestPoolFromEnv opens and pings a pool for envVar, skipping the test when
// the DSN is unset. The pool closes on cleanup.
func openTestPoolFromEnv(t *testing.T, envVar string) *sql.DB {
	t.Helper()

	dsn := envOrSkip(t, envVar)
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open(%s): %v", envVar, err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		t.Fatalf("ping %s: %v", envVar, err)
	}
	return pool
}

func envOrSkip(t *testing.T, envVar string) string {
	t.Helper()
	dsn := os.Getenv(envVar)
	if dsn == "" {
		t.Skipf("%s not set; skipping action integration test", envVar)
	}
	return dsn
}

// --- Scenario #1: action role CAN insert a company through the approved function.

// TestAddCompanyHandler_ActionRoleInsertsAndIsIdempotent drives the real
// add_company handler against the action pool. It asserts the returned row and
// that inserted flips true -> false on a repeated identical call (#1), and that
// exactly one company row exists for the unique board token (#7).
func TestAddCompanyHandler_ActionRoleInsertsAndIsIdempotent(t *testing.T) {
	owner, readOnly, action := openActionTestPools(t)

	boardToken := uniqueToken("mcp-action-add")
	t.Cleanup(func() { deleteCompanyByBoardToken(t, owner, boardToken) })

	handler := addCompanyHandler(action)
	req := newCallToolRequest(map[string]any{
		"name":        "MCP Action Test Co",
		"ats":         "greenhouse",
		"board_token": boardToken,
		"probe":       false, // no live HTTP in the boundary test
	})

	// First call: inserted=true.
	first := callAddCompany(t, handler, req)
	if !first.Ok {
		t.Fatalf("first add_company ok=false; errors=%+v", first.Errors)
	}
	if !first.Inserted {
		t.Fatalf("first call inserted=false, want true")
	}
	if first.Company == nil || first.Company.BoardToken != boardToken || first.Company.ATS != "greenhouse" {
		t.Fatalf("first company = %+v, want board_token=%q ats=greenhouse", first.Company, boardToken)
	}
	if first.Company.ID == 0 {
		t.Fatalf("first company id = 0, want a real id")
	}

	// Second identical call: same row, inserted=false (on-conflict no-op).
	second := callAddCompany(t, handler, req)
	if !second.Ok {
		t.Fatalf("second add_company ok=false; errors=%+v", second.Errors)
	}
	if second.Inserted {
		t.Fatalf("second call inserted=true, want false (idempotent)")
	}
	if second.Company == nil || second.Company.ID != first.Company.ID {
		t.Fatalf("second company id = %v, want same id as first %d", second.Company, first.Company.ID)
	}

	// #7: exactly one row for this board token, scoped to test-specific data.
	if got := countCompaniesByToken(t, readOnly, boardToken); got != 1 {
		t.Fatalf("company rows for board_token %q = %d, want exactly 1", boardToken, got)
	}
}

// TestRecordUnsupportedCompanyHandler_ActionRoleInsertsAndRefreshes drives the
// registry write path through the action pool. A normalized-name conflict must
// retain the first display name and timestamp while replacing the current
// observation fields.
func TestRecordUnsupportedCompanyHandler_ActionRoleInsertsAndRefreshes(t *testing.T) {
	owner, _, action := openActionTestPools(t)

	name := "MCP Unsupported Registry " + uniqueToken("co")
	t.Cleanup(func() {
		if _, err := owner.ExecContext(context.Background(),
			"DELETE FROM unsupported_companies WHERE name = $1", name); err != nil {
			t.Errorf("cleanup unsupported company %q: %v", name, err)
		}
	})

	handler := recordUnsupportedCompanyHandler(action)
	first := callRecordUnsupportedCompany(t, handler, newCallToolRequest(map[string]any{
		"name":              name,
		"url":               "https://unsupported.example/careers",
		"detected_platform": "rippling",
		"reason":            "unsupported_ats",
	}))
	if !first.Ok || first.UnsupportedCompany == nil {
		t.Fatalf("first record_unsupported_company = %+v, want success", first)
	}
	if first.UnsupportedCompany.URL == nil || *first.UnsupportedCompany.URL != "https://unsupported.example/careers" || first.UnsupportedCompany.DetectedPlatform == nil || *first.UnsupportedCompany.DetectedPlatform != "rippling" {
		t.Fatalf("first unsupported_company = %+v, want url and platform", first.UnsupportedCompany)
	}

	secondName := strings.ToLower(name) + "!!!"
	second := callRecordUnsupportedCompany(t, handler, newCallToolRequest(map[string]any{
		"name":   secondName,
		"reason": "no_careers",
	}))
	if !second.Ok || second.UnsupportedCompany == nil {
		t.Fatalf("second record_unsupported_company = %+v, want success", second)
	}
	if second.UnsupportedCompany.ID != first.UnsupportedCompany.ID || second.UnsupportedCompany.Name != name {
		t.Fatalf("conflict row = %+v, want original id/name %d/%q", second.UnsupportedCompany, first.UnsupportedCompany.ID, name)
	}
	if second.UnsupportedCompany.Reason != "no_careers" || second.UnsupportedCompany.URL != nil || second.UnsupportedCompany.DetectedPlatform != nil {
		t.Fatalf("refreshed row = %+v, want overwritten no_careers metadata", second.UnsupportedCompany)
	}

	var firstSeen, lastChecked time.Time
	if err := owner.QueryRowContext(t.Context(),
		"SELECT first_seen_at, last_checked_at FROM unsupported_companies WHERE id = $1", first.UnsupportedCompany.ID).
		Scan(&firstSeen, &lastChecked); err != nil {
		t.Fatalf("read refreshed unsupported company: %v", err)
	}
	if firstSeen.UTC().Format(time.RFC3339) != first.UnsupportedCompany.FirstSeenAt {
		t.Fatalf("first_seen_at = %s, want preserved %s", firstSeen, first.UnsupportedCompany.FirstSeenAt)
	}
	if !lastChecked.After(firstSeen) {
		t.Fatalf("last_checked_at = %s, want after first_seen_at %s", lastChecked, firstSeen)
	}
}

// TestRecordUnsupportedCompanyFunction_ActionRoleRejectsInvalidInputs calls the
// approved function directly through the action DSN. The database boundary must
// preserve registry invariants even when a caller bypasses the Go handler.
func TestRecordUnsupportedCompanyFunction_ActionRoleRejectsInvalidInputs(t *testing.T) {
	owner, _, action := openActionTestPools(t)

	namePrefix := "MCP Invalid Unsupported " + uniqueToken("co")
	t.Cleanup(func() {
		if _, err := owner.ExecContext(context.Background(),
			"DELETE FROM unsupported_companies WHERE name = $1 OR name LIKE $2",
			" \t\n", namePrefix+"%"); err != nil {
			t.Errorf("cleanup invalid unsupported-company calls: %v", err)
		}
	})

	tests := []struct {
		name   string
		input  string
		url    any
		reason string
	}{
		{name: "blank name", input: " \t\n", url: nil, reason: "no_careers"},
		{name: "invalid reason", input: namePrefix + " invalid reason", url: nil, reason: "other"},
		{name: "unsupported ATS missing URL", input: namePrefix + " missing URL", url: nil, reason: "unsupported_ats"},
		{name: "unsupported ATS blank URL", input: namePrefix + " blank URL", url: " \t", reason: "unsupported_ats"},
		{name: "unsupported ATS relative URL", input: namePrefix + " relative URL", url: "/careers", reason: "unsupported_ats"},
		{name: "unsupported ATS malformed URL", input: namePrefix + " malformed URL", url: "https://example.com:bad/careers", reason: "unsupported_ats"},
		{name: "no careers malformed supplied URL", input: namePrefix + " no careers malformed URL", url: "ftp://example.com/careers", reason: "no_careers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id int64
			err := action.QueryRowContext(t.Context(),
				"SELECT id FROM mcp.record_unsupported_company($1, $2, $3, $4)",
				tt.input, tt.url, nil, tt.reason).Scan(&id)
			assertSQLState(t, err, "22023")

			var count int64
			if err := owner.QueryRowContext(t.Context(),
				"SELECT count(*) FROM unsupported_companies WHERE name = $1", tt.input).Scan(&count); err != nil {
				t.Fatalf("count rejected unsupported company: %v", err)
			}
			if count != 0 {
				t.Fatalf("rejected call persisted %d rows, want 0", count)
			}
		})
	}
}

// --- Scenarios #2 and #8: action role CAN save classifications through the
// approved function, append-only.

// TestSaveEnrichmentHandler_ActionRoleAppendsWithoutMutatingHistory drives the
// real save_enrichment handler. It seeds a posting plus a PRIOR classification,
// then saves two new classifications through the action pool and asserts:
//   - #2: the action role can save a classification through the approved path.
//   - #8: each call inserts exactly one new classifications row (count +1/call)
//     and the seeded prior row (id, classified_at, prompt_version) is unchanged.
func TestSaveEnrichmentHandler_ActionRoleAppendsWithoutMutatingHistory(t *testing.T) {
	owner, readOnly, action := openActionTestPools(t)

	postingID := seedPosting(t, owner)
	prior := seedPriorClassification(t, owner, postingID)

	handler := saveEnrichmentHandler(readOnly, action)

	// Use only seeded taxonomy slugs so the call mints nothing and cannot collide
	// with shared taxonomy. software-engineer (canonical role, engineering
	// dimension) and typescript (skill) are seeded by migration 000001.
	makeReq := func(notes string) mcp.CallToolRequest {
		return newCallToolRequest(map[string]any{
			"posting_id":     postingID,
			"provenance":     map[string]any{"model": "mcp-test", "prompt_version": "mcp-test-v1"},
			"classification": map[string]any{"seniority": "senior", "notes": notes},
			"canonical_roles": []any{
				map[string]any{"slug": "software-engineer", "name": "Software Engineer", "dimensions": []any{"engineering"}},
			},
			"skills": []any{
				map[string]any{"slug": "typescript", "name": "TypeScript"},
			},
			"summary": "integration test classification",
		})
	}

	base := countClassifications(t, readOnly, postingID) // includes the seeded prior row

	// First save: count increases by exactly one.
	env1 := callSaveEnrichment(t, handler, makeReq("first save"))
	if !env1.Ok {
		t.Fatalf("first save_enrichment ok=false; errors=%+v", env1.Errors)
	}
	if env1.ClassificationID == nil || *env1.ClassificationID == 0 {
		t.Fatalf("first save classification_id = %v, want a real id", env1.ClassificationID)
	}
	if got := countClassifications(t, readOnly, postingID); got != base+1 {
		t.Fatalf("classifications after first save = %d, want %d", got, base+1)
	}

	// Second save: a NEW row, count increases by one again. Append-only.
	env2 := callSaveEnrichment(t, handler, makeReq("second save"))
	if !env2.Ok {
		t.Fatalf("second save_enrichment ok=false; errors=%+v", env2.Errors)
	}
	if env2.ClassificationID == nil || *env2.ClassificationID == *env1.ClassificationID {
		t.Fatalf("second classification_id = %v, want a distinct new id (first was %d)", env2.ClassificationID, *env1.ClassificationID)
	}
	if got := countClassifications(t, readOnly, postingID); got != base+2 {
		t.Fatalf("classifications after second save = %d, want %d", got, base+2)
	}

	// #8: the seeded prior classification is untouched — no mutation of history.
	after := readClassification(t, readOnly, prior.id)
	if !after.classifiedAt.Equal(prior.classifiedAt) {
		t.Fatalf("prior classification classified_at changed: was %v, now %v", prior.classifiedAt, after.classifiedAt)
	}
	if after.promptVersion != prior.promptVersion {
		t.Fatalf("prior classification prompt_version changed: was %q, now %q", prior.promptVersion, after.promptVersion)
	}
	if after.seniority != prior.seniority {
		t.Fatalf("prior classification seniority changed: was %q, now %q", prior.seniority, after.seniority)
	}
}

// --- Scenario #3: action role CANNOT write directly to data/taxonomy tables or
// run DDL. Direct SQL through the action DSN; every attempt must be denied.

func TestActionRole_DirectTableWritesAndDDLDenied(t *testing.T) {
	_, _, action := openActionTestPools(t)

	tests := []struct {
		name string
		sql  string
	}{
		{"posting_snapshots", "INSERT INTO posting_snapshots (job_posting_id, fetched_at, raw_data) VALUES (1, now(), '{}'::jsonb)"},
		{"job_postings", "INSERT INTO job_postings (company_id, source_type, source_url) VALUES (1, 'ats', 'https://example.test/denied')"},
		{"classifications", "INSERT INTO classifications (job_posting_id, model, prompt_version, seniority) VALUES (1, 'm', 'v', 'mid')"},
		{"canonical_roles", "INSERT INTO canonical_roles (slug, name) VALUES ('denied-role', 'Denied')"},
		{"specializations", "INSERT INTO specializations (slug, name) VALUES ('denied-spec', 'Denied')"},
		{"skills", "INSERT INTO skills (slug, name) VALUES ('denied-skill', 'Denied')"},
		{"role_dimensions", "INSERT INTO role_dimensions (slug, name) VALUES ('denied-dim', 'Denied')"},
		{"canonical_role_dimensions", "INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id) VALUES (1, 1)"},
		{"job_posting_roles", "INSERT INTO job_posting_roles (classification_id, role_id) VALUES (1, 1)"},
		{"job_posting_specializations", "INSERT INTO job_posting_specializations (classification_id, specialization_id) VALUES (1, 1)"},
		{"job_posting_skills", "INSERT INTO job_posting_skills (classification_id, skill_id) VALUES (1, 1)"},
		{"DDL create table", "CREATE TABLE public.action_denied_evil (id bigint)"},
		{"DDL alter table", "ALTER TABLE public.companies ADD COLUMN evil text"},
		{"DDL drop function", "DROP FUNCTION mcp.add_company(text, text, text, text, text)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := action.ExecContext(t.Context(), tt.sql)
			assertPermissionDenied(t, err)
		})
	}
}

// --- Scenario #4: action role CANNOT directly INSERT/UPDATE/DELETE companies.

func TestActionRole_DirectCompaniesWritesDenied(t *testing.T) {
	_, _, action := openActionTestPools(t)

	tests := []struct {
		name string
		sql  string
	}{
		{"insert", "INSERT INTO companies (name, ats, board_token) VALUES ('Denied', 'greenhouse', 'denied-direct')"},
		{"update", "UPDATE companies SET name = 'Denied' WHERE id = 1"},
		{"delete", "DELETE FROM companies WHERE id = 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := action.ExecContext(t.Context(), tt.sql)
			assertPermissionDenied(t, err)
		})
	}
}

// --- Scenario #5: action role CANNOT CREATE TEMP TABLE.

func TestActionRole_TempTableCreationDenied(t *testing.T) {
	_, _, action := openActionTestPools(t)

	suffix := time.Now().UnixNano()
	tests := []struct {
		name string
		sql  string
	}{
		{"create temp table", fmt.Sprintf("CREATE TEMP TABLE action_temp_%d (id bigint)", suffix)},
		{"select into temp", fmt.Sprintf("SELECT 1::bigint AS id INTO TEMP action_select_into_%d", suffix)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := action.ExecContext(t.Context(), tt.sql)
			assertPermissionDenied(t, err)
		})
	}
}

// --- Scenario #6: read-only role still cannot run writes.

func TestReadOnlyRole_WritesDenied(t *testing.T) {
	_, readOnly, _ := openActionTestPools(t)

	_, err := readOnly.ExecContext(t.Context(),
		"INSERT INTO companies (name, ats, board_token) VALUES ('RO Denied', 'greenhouse', 'ro-denied-direct')")
	assertPermissionDenied(t, err)
}

// --- Scenario: action role CANNOT directly SELECT application tables. The action
// role reaches data only through approved mcp.* functions; the read side of the
// boundary must be locked down too. A bare SELECT must fail with permission
// denied (SQLSTATE 42501), proving no leftover table-read grant.

func TestActionRole_DirectTableReadsDenied(t *testing.T) {
	_, _, action := openActionTestPools(t)

	tests := []struct {
		name string
		sql  string
	}{
		{"companies", "SELECT count(*) FROM public.companies"},
		{"job_postings", "SELECT count(*) FROM public.job_postings"},
		{"classifications", "SELECT count(*) FROM public.classifications"},
		{"canonical_roles", "SELECT count(*) FROM public.canonical_roles"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := action.ExecContext(t.Context(), tt.sql)
			assertPermissionDenied(t, err)
		})
	}
}

// --- Helpers.

// assertPermissionDenied requires a non-nil error whose text reports a Postgres
// permission denial (SQLSTATE 42501). The action role's whole guarantee is that
// these statements fail with insufficient privilege.
func assertPermissionDenied(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("statement succeeded, want permission denied")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "permission denied") && !strings.Contains(msg, "42501") {
		t.Fatalf("error = %q, want a permission-denied (SQLSTATE 42501) failure", err.Error())
	}
}

func assertSQLState(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("statement succeeded, want SQLSTATE %s", want)
	}

	var sqlStateError interface{ SQLState() string }
	if !errors.As(err, &sqlStateError) {
		t.Fatalf("error = %q, want SQLSTATE %s", err.Error(), want)
	}
	if got := sqlStateError.SQLState(); got != want {
		t.Fatalf("SQLSTATE = %s, want %s (error: %q)", got, want, err.Error())
	}
}

// newCallToolRequest builds a CallToolRequest carrying the given arguments,
// matching what the MCP transport hands a tool handler. BindArguments
// JSON-roundtrips Params.Arguments, so a map[string]any populates the typed DTO.
func newCallToolRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

// callAddCompany invokes the handler and decodes its JSON text envelope. A
// transport-level error or a non-text result is a test failure: the action tools
// return failures inside the envelope, never as transport errors.
func callAddCompany(t *testing.T, handler server.ToolHandlerFunc, req mcp.CallToolRequest) addCompanyEnvelope {
	t.Helper()
	var env addCompanyEnvelope
	decodeHandlerResult(t, handler, req, &env)
	return env
}

func callRecordUnsupportedCompany(t *testing.T, handler server.ToolHandlerFunc, req mcp.CallToolRequest) recordUnsupportedCompanyEnvelope {
	t.Helper()
	var env recordUnsupportedCompanyEnvelope
	decodeHandlerResult(t, handler, req, &env)
	return env
}

// callSaveEnrichment invokes the handler and decodes its JSON text envelope.
func callSaveEnrichment(t *testing.T, handler server.ToolHandlerFunc, req mcp.CallToolRequest) saveEnrichmentEnvelope {
	t.Helper()
	var env saveEnrichmentEnvelope
	decodeHandlerResult(t, handler, req, &env)
	return env
}

// decodeHandlerResult runs a tool handler func and unmarshals its single text
// content into out.
func decodeHandlerResult(t *testing.T, fn server.ToolHandlerFunc, req mcp.CallToolRequest, out any) {
	t.Helper()
	res, err := fn(t.Context(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if res == nil || len(res.Content) != 1 {
		t.Fatalf("handler result = %+v, want exactly one content block", res)
	}
	text, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("handler content = %T, want TextContent", res.Content[0])
	}
	if err := json.Unmarshal([]byte(text.Text), out); err != nil {
		t.Fatalf("decoding handler envelope %q: %v", text.Text, err)
	}
}

func uniqueToken(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func deleteCompanyByBoardToken(t *testing.T, owner *sql.DB, boardToken string) {
	t.Helper()
	if _, err := owner.ExecContext(context.Background(),
		"DELETE FROM companies WHERE board_token = $1", boardToken); err != nil {
		t.Errorf("cleanup company %q: %v", boardToken, err)
	}
}

func countCompaniesByToken(t *testing.T, pool *sql.DB, boardToken string) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRowContext(t.Context(),
		"SELECT count(*) FROM companies WHERE board_token = $1", boardToken).Scan(&n); err != nil {
		t.Fatalf("count companies by token: %v", err)
	}
	return n
}

// seedPosting inserts a throwaway company + job_posting via the owner DSN and
// returns the posting id. Cleanup deletes the company; the FK cascade
// (classifications ON DELETE CASCADE, job_postings ON DELETE RESTRICT) requires
// removing postings and classifications first, so cleanup deletes in dependency
// order.
func seedPosting(t *testing.T, owner *sql.DB) int64 {
	t.Helper()
	ctx := context.Background()
	boardToken := uniqueToken("mcp-save-enrich")

	var companyID int64
	if err := owner.QueryRowContext(ctx,
		"INSERT INTO companies (name, ats, board_token) VALUES ($1, 'greenhouse', $2) RETURNING id",
		"MCP Save Enrich Co", boardToken).Scan(&companyID); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	var postingID int64
	sourceURL := "https://example.test/" + boardToken
	if err := owner.QueryRowContext(ctx,
		"INSERT INTO job_postings (company_id, source_type, source_url) VALUES ($1, 'ats', $2) RETURNING id",
		companyID, sourceURL).Scan(&postingID); err != nil {
		t.Fatalf("seed job_posting: %v", err)
	}

	t.Cleanup(func() {
		// classifications cascade from job_postings; job_postings RESTRICT against
		// companies, so delete postings (and their classifications) before company.
		if _, err := owner.ExecContext(context.Background(),
			"DELETE FROM classifications WHERE job_posting_id = $1", postingID); err != nil {
			t.Errorf("cleanup classifications for posting %d: %v", postingID, err)
		}
		if _, err := owner.ExecContext(context.Background(),
			"DELETE FROM job_postings WHERE id = $1", postingID); err != nil {
			t.Errorf("cleanup job_posting %d: %v", postingID, err)
		}
		if _, err := owner.ExecContext(context.Background(),
			"DELETE FROM companies WHERE id = $1", companyID); err != nil {
			t.Errorf("cleanup company %d: %v", companyID, err)
		}
	})

	return postingID
}

type seededClassification struct {
	id            int64
	classifiedAt  time.Time
	promptVersion string
	seniority     string
}

// seedPriorClassification inserts a prior classification row for the posting via
// the owner DSN. #8 asserts this row is unchanged after later save_enrichment
// calls. Its classified_at is set explicitly in the past so any accidental
// rewrite to now() would be detectable.
func seedPriorClassification(t *testing.T, owner *sql.DB, postingID int64) seededClassification {
	t.Helper()
	priorAt := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	c := seededClassification{
		classifiedAt:  priorAt,
		promptVersion: "prior-seed-v0",
		seniority:     "mid",
	}
	if err := owner.QueryRowContext(context.Background(),
		`INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority, notes)
		 VALUES ($1, 'seed-model', $2, $3, $4, 'seeded prior') RETURNING id`,
		postingID, c.promptVersion, c.classifiedAt, c.seniority).Scan(&c.id); err != nil {
		t.Fatalf("seed prior classification: %v", err)
	}
	return c
}

func countClassifications(t *testing.T, pool *sql.DB, postingID int64) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRowContext(t.Context(),
		"SELECT count(*) FROM classifications WHERE job_posting_id = $1", postingID).Scan(&n); err != nil {
		t.Fatalf("count classifications: %v", err)
	}
	return n
}

func readClassification(t *testing.T, pool *sql.DB, id int64) seededClassification {
	t.Helper()
	var c seededClassification
	c.id = id
	if err := pool.QueryRowContext(t.Context(),
		"SELECT classified_at, prompt_version, seniority FROM classifications WHERE id = $1", id).
		Scan(&c.classifiedAt, &c.promptVersion, &c.seniority); err != nil {
		t.Fatalf("read classification %d: %v", id, err)
	}
	return c
}
