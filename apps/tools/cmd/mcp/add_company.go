// add_company is an MCP action tool: it inserts a company through the
// approved mcp.add_company SECURITY DEFINER function, run by the locked-down
// action role. The agent sends only typed parameters — never SQL — and the
// server validates them, optionally probes the live ATS board, then calls the
// fixed parameterized statement against the action pool.
// See: agent-context/lib/developer-guide.md §5.7 (Database access)
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/ats"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/atsdetect"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/domain"
)

// Action error codes. Each maps to a stable path the agent can branch on; the
// envelope returns these in errors[] rather than as an MCP transport error.
const (
	codeMissingRequired   = "missing_required"
	codeUnsupportedATS    = "unsupported_ats"
	codeInvalidBoardToken = "invalid_board_token"
	codeInvalidURL        = "invalid_url"
	codeProbeFailed       = "probe_failed"
	codeDBError           = "db_error"
)

// seedFilePath is the canonical companies seed. A direct MCP insert writes a
// DB row without touching this file, so the success follow-up names it to keep
// the drift visible to the operator. See agent-context/lib/watchlist.md for the
// workflow that governs how companies enter the seed.
const seedFilePath = "apps/tools/internal/db/seeds/companies.sql"

// addCompanyRequest is the MCP tool DTO. JSON keys are the wire contract with
// the agent; field names follow Go style. Probe is a pointer so an omitted
// probe (default true) is distinguishable from an explicit false.
type addCompanyRequest struct {
	Name           string `json:"name"`
	ATS            string `json:"ats"`
	BoardToken     string `json:"board_token"`
	Industry       string `json:"industry"`
	CareersPageURL string `json:"careers_page_url"`
	Probe          *bool  `json:"probe"`
}

// actionError is the shared action error shape returned in the envelope's
// errors[] array.
type actionError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// companyDTO is the company portion of the mcp.add_company response — the
// seven columns the caller receives; `inserted` is reported separately on the
// envelope. Industry and CareersPageURL are pointers so an absent value
// marshals as JSON null rather than an empty string.
type companyDTO struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	ATS            string  `json:"ats"`
	BoardToken     string  `json:"board_token"`
	CreatedAt      string  `json:"created_at"`
	Industry       *string `json:"industry"`
	CareersPageURL *string `json:"careers_page_url"`
}

// probeResult reports the outcome of the live ATS probe. Attempted is false
// only when probe=false; Valid is true when FetchPostings succeeds (an empty
// board is valid). Error carries the failure string when Valid is false.
type probeResult struct {
	ATS           string `json:"ats"`
	BoardToken    string `json:"board_token"`
	Attempted     bool   `json:"attempted"`
	Valid         bool   `json:"valid"`
	PostingsCount int    `json:"postings_count"`
	Error         string `json:"error,omitempty"`
}

// addCompanyEnvelope is the JSON the tool returns. Validation, probe, and DB
// failures set Ok=false here — they are not MCP transport errors. SeedFileUpdated
// is always false: MCP never edits source files as a side effect of a DB action.
// On success FollowUp names the seed file so the operator knows the row is not
// yet canonical; failures leave it null.
type addCompanyEnvelope struct {
	Ok              bool          `json:"ok"`
	Inserted        bool          `json:"inserted"`
	Company         *companyDTO   `json:"company"`
	SeedFileUpdated bool          `json:"seed_file_updated"`
	FollowUp        *string       `json:"follow_up"`
	ProbeResult     *probeResult  `json:"probe_result"`
	Errors          []actionError `json:"errors"`
}

// atsProbe is the minimal slice of an ATS adapter the action tool needs to
// verify a board token. The fetcher's adapters satisfy it implicitly.
type atsProbe interface {
	FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error)
}

// probeFactory builds a live ATS adapter for a supported ats value. Injectable
// so tests can fake the probe without real HTTP. The caller has already
// validated ats through atsdetect.
type probeFactory func(atsName string) (atsProbe, error)

// addCompanyExecutor runs the approved mcp.add_company function and returns the
// eight scanned columns. Injectable so tests can supply canned rows without a
// database. The production implementation binds the action pool.
type addCompanyExecutor interface {
	addCompany(ctx context.Context, params addCompanyParams) (addCompanyRow, error)
}

// addCompanyParams carries the five function arguments. Industry and
// CareersPageURL are sql.NullString so an omitted value is passed as SQL NULL,
// not an empty string.
type addCompanyParams struct {
	Name           string
	ATS            string
	BoardToken     string
	Industry       sql.NullString
	CareersPageURL sql.NullString
}

// addCompanyRow holds the eight columns mcp.add_company returns. Nullable
// columns use sql.Null* so absence is distinguishable from empty.
type addCompanyRow struct {
	ID             int64
	Name           string
	ATS            string
	BoardToken     string
	CreatedAt      sql.NullTime
	Industry       sql.NullString
	CareersPageURL sql.NullString
	Inserted       bool
}

// poolExecutor runs mcp.add_company against the action pool with a fixed,
// fully-parameterized statement. The statement is constant; every value is a
// bound parameter, so there is no injection surface even though it bypasses
// sqlc (see developer-guide §5.7).
type poolExecutor struct {
	pool *sql.DB
}

const addCompanySQL = `SELECT id, name, ats, board_token, created_at, industry, careers_page_url, inserted ` +
	`FROM mcp.add_company($1, $2, $3, $4, $5)`

func (e poolExecutor) addCompany(ctx context.Context, params addCompanyParams) (addCompanyRow, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	var row addCompanyRow
	err := e.pool.QueryRowContext(queryCtx, addCompanySQL,
		params.Name,
		params.ATS,
		params.BoardToken,
		params.Industry,
		params.CareersPageURL,
	).Scan(
		&row.ID,
		&row.Name,
		&row.ATS,
		&row.BoardToken,
		&row.CreatedAt,
		&row.Industry,
		&row.CareersPageURL,
		&row.Inserted,
	)
	if err != nil {
		return addCompanyRow{}, fmt.Errorf("calling mcp.add_company: %w", err)
	}
	return row, nil
}

// defaultProbeFactory constructs the live adapter for ats using a shared HTTP
// client. ats is assumed already validated through atsdetect.
func defaultProbeFactory(atsName string) (atsProbe, error) {
	client := &http.Client{}
	switch atsName {
	case "greenhouse":
		return ats.NewGreenhouse(client), nil
	case "lever":
		return ats.NewLever(client), nil
	case "ashby":
		return ats.NewAshby(client), nil
	case "workday":
		return ats.NewWorkday(client), nil
	case "workable":
		return ats.NewWorkable(client), nil
	default:
		return nil, fmt.Errorf("no adapter for ats %q", atsName)
	}
}

// addCompanyHandler wires the tool to the action pool and the live probe
// factory. The action pool is intentionally the only DB handle this tool gets.
func addCompanyHandler(pool *sql.DB) server.ToolHandlerFunc {
	return addCompanyHandlerWithDeps(poolExecutor{pool: pool}, defaultProbeFactory)
}

// addCompanyHandlerWithDeps is the testable handler body. Tests inject a fake
// executor and probe factory; production wiring passes the action pool and the
// live factory via addCompanyHandler.
func addCompanyHandlerWithDeps(exec addCompanyExecutor, makeProbe probeFactory) server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req addCompanyRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			// A request the server cannot decode is a genuinely malformed tool
			// call — the one case that warrants an MCP transport error.
			return mcp.NewToolResultError(fmt.Sprintf("decoding add_company arguments: %v", err)), nil
		}

		env := runAddCompany(ctx, req, exec, makeProbe)
		payload, err := json.Marshal(env)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding add_company result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

// runAddCompany validates the request, optionally probes the ATS, and on
// success inserts via the approved function. Every failure mode returns an
// ok=false envelope; it never returns an MCP transport error.
func runAddCompany(ctx context.Context, req addCompanyRequest, exec addCompanyExecutor, makeProbe probeFactory) addCompanyEnvelope {
	name := strings.TrimSpace(req.Name)
	atsName := strings.TrimSpace(req.ATS)
	boardToken := strings.TrimSpace(req.BoardToken)
	industry := strings.TrimSpace(req.Industry)
	careersURL := strings.TrimSpace(req.CareersPageURL)

	if errs := validateAddCompany(name, atsName, boardToken, careersURL); len(errs) > 0 {
		return failureEnvelope(errs, nil)
	}

	// probe defaults to true when omitted; an explicit false skips the live call.
	doProbe := req.Probe == nil || *req.Probe
	var probe *probeResult
	if doProbe {
		probe = runProbe(ctx, atsName, boardToken, makeProbe)
		if !probe.Valid {
			return failureEnvelope([]actionError{{
				Path:    "probe",
				Code:    codeProbeFailed,
				Message: probe.Error,
			}}, probe)
		}
	} else {
		probe = &probeResult{ATS: atsName, BoardToken: boardToken, Attempted: false}
	}

	row, err := exec.addCompany(ctx, addCompanyParams{
		Name:           name,
		ATS:            atsName,
		BoardToken:     boardToken,
		Industry:       nullStringFrom(industry),
		CareersPageURL: nullStringFrom(careersURL),
	})
	if err != nil {
		return failureEnvelope([]actionError{{
			Path:    "db",
			Code:    codeDBError,
			Message: err.Error(),
		}}, probe)
	}

	company := mapCompanyRow(row)
	followUp := seedDriftFollowUp()
	return addCompanyEnvelope{
		Ok:              true,
		Inserted:        row.Inserted,
		Company:         &company,
		SeedFileUpdated: false,
		FollowUp:        &followUp,
		ProbeResult:     probe,
		Errors:          []actionError{},
	}
}

// seedDriftFollowUp returns the success follow-up message. The DB row now exists
// but the canonical seed file does not reflect it; MCP deliberately does not edit
// source files as a DB side effect, so a human-reviewed step is required to keep
// the seed authoritative. A future stage_company_seed_patch action will write the
// matching seed SQL for that review — until it exists, this message is the seam.
func seedDriftFollowUp() string {
	return fmt.Sprintf(
		"Company row written to the database but not yet reflected in the canonical seed file %s. "+
			"A human-reviewed step is needed to keep the seed authoritative; a future stage_company_seed_patch action will draft that seed SQL for review.",
		seedFilePath,
	)
}

// validateAddCompany checks the trimmed request fields and returns one
// actionError per violation. An empty slice means the request is valid.
func validateAddCompany(name, atsName, boardToken, careersURL string) []actionError {
	var errs []actionError

	if name == "" {
		errs = append(errs, actionError{Path: "name", Code: codeMissingRequired, Message: "name is required"})
	}
	if atsName == "" {
		errs = append(errs, actionError{Path: "ats", Code: codeMissingRequired, Message: "ats is required"})
	} else if err := atsdetect.ValidateATS(atsName); err != nil {
		errs = append(errs, actionError{Path: "ats", Code: codeUnsupportedATS, Message: err.Error()})
	}
	if boardToken == "" {
		errs = append(errs, actionError{Path: "board_token", Code: codeMissingRequired, Message: "board_token is required"})
	}

	// ATS-specific board_token syntax. Only checked when both ats and token are
	// present and the ats is supported — otherwise the errors above already
	// describe the problem.
	if boardToken != "" && atsdetect.ValidateATS(atsName) == nil {
		if err := atsdetect.ValidateBoardToken(atsName, boardToken); err != nil {
			errs = append(errs, actionError{Path: "board_token", Code: codeInvalidBoardToken, Message: err.Error()})
		}
	}

	if careersURL != "" {
		if err := atsdetect.ValidateURL(careersURL); err != nil {
			errs = append(errs, actionError{Path: "careers_page_url", Code: codeInvalidURL, Message: fieldURLMessage("careers_page_url", err)})
		}
	}

	return errs
}

// fieldURLMessage keeps add_company's field-specific validation wording while
// delegating URL parsing to atsdetect.
func fieldURLMessage(field string, err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "url ") {
		return field + strings.TrimPrefix(msg, "url")
	}
	return msg
}

// runProbe constructs the live adapter and fetches the board. A construction or
// fetch failure yields an invalid probe result with the error string; an empty
// board is valid. PostingsCount is reported only when valid.
func runProbe(ctx context.Context, atsName, boardToken string, makeProbe probeFactory) *probeResult {
	result := &probeResult{ATS: atsName, BoardToken: boardToken, Attempted: true}

	adapter, err := makeProbe(atsName)
	if err != nil {
		result.Error = fmt.Sprintf("constructing %s adapter: %v", atsName, err)
		return result
	}

	postings, err := adapter.FetchPostings(ctx, boardToken)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Valid = true
	result.PostingsCount = len(postings)
	return result
}

// failureEnvelope builds the ok=false envelope shared by every failure path.
// probe is the probe result to surface (nil unless a probe ran).
func failureEnvelope(errs []actionError, probe *probeResult) addCompanyEnvelope {
	return addCompanyEnvelope{
		Ok:              false,
		Inserted:        false,
		Company:         nil,
		SeedFileUpdated: false,
		FollowUp:        nil,
		ProbeResult:     probe,
		Errors:          errs,
	}
}

// mapCompanyRow maps the eight scanned columns to the response DTO. created_at
// is rendered RFC3339; industry and careers_page_url become nil pointers when
// the column is NULL.
func mapCompanyRow(row addCompanyRow) companyDTO {
	dto := companyDTO{
		ID:             row.ID,
		Name:           row.Name,
		ATS:            row.ATS,
		BoardToken:     row.BoardToken,
		Industry:       nullableString(row.Industry),
		CareersPageURL: nullableString(row.CareersPageURL),
	}
	if row.CreatedAt.Valid {
		dto.CreatedAt = row.CreatedAt.Time.UTC().Format(time.RFC3339)
	}
	return dto
}

// nullStringFrom converts a trimmed string into a sql.NullString: empty means
// SQL NULL, so omitted optional fields are not stored as empty strings.
func nullStringFrom(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
