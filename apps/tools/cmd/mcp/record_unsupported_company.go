package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/atsdetect"
)

const (
	unsupportedCompanyReasonUnsupportedATS = "unsupported_ats"
	unsupportedCompanyReasonNoCareers      = "no_careers"
	codeInvalidUnsupportedCompanyReason    = "invalid_reason"
)

type recordUnsupportedCompanyRequest struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	DetectedPlatform string `json:"detected_platform"`
	Reason           string `json:"reason"`
}

type unsupportedCompanyDTO struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	URL              *string `json:"url"`
	DetectedPlatform *string `json:"detected_platform"`
	Reason           string  `json:"reason"`
	FirstSeenAt      string  `json:"first_seen_at"`
	LastCheckedAt    string  `json:"last_checked_at"`
}

type recordUnsupportedCompanyEnvelope struct {
	Ok                 bool                   `json:"ok"`
	UnsupportedCompany *unsupportedCompanyDTO `json:"unsupported_company"`
	Errors             []actionError          `json:"errors"`
}

type recordUnsupportedCompanyParams struct {
	Name             string
	URL              sql.NullString
	DetectedPlatform sql.NullString
	Reason           string
}

type recordUnsupportedCompanyRow struct {
	ID               int64
	Name             string
	URL              sql.NullString
	DetectedPlatform sql.NullString
	Reason           string
	FirstSeenAt      time.Time
	LastCheckedAt    time.Time
}

type recordUnsupportedCompanyExecutor interface {
	recordUnsupportedCompany(context.Context, recordUnsupportedCompanyParams) (recordUnsupportedCompanyRow, error)
}

type recordUnsupportedCompanyPoolExecutor struct {
	pool *sql.DB
}

// This fixed, fully-parameterized QueryRowContext call bypasses sqlc: its offline parser cannot expand this multi-column RETURNS TABLE result.
const recordUnsupportedCompanySQL = `SELECT id, name, url, detected_platform, reason, first_seen_at, last_checked_at ` +
	`FROM mcp.record_unsupported_company($1, $2, $3, $4)`

func (e recordUnsupportedCompanyPoolExecutor) recordUnsupportedCompany(ctx context.Context, params recordUnsupportedCompanyParams) (recordUnsupportedCompanyRow, error) {
	queryCtx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	var row recordUnsupportedCompanyRow
	err := e.pool.QueryRowContext(queryCtx, recordUnsupportedCompanySQL,
		params.Name,
		params.URL,
		params.DetectedPlatform,
		params.Reason,
	).Scan(
		&row.ID,
		&row.Name,
		&row.URL,
		&row.DetectedPlatform,
		&row.Reason,
		&row.FirstSeenAt,
		&row.LastCheckedAt,
	)
	if err != nil {
		return recordUnsupportedCompanyRow{}, fmt.Errorf("calling mcp.record_unsupported_company: %w", err)
	}
	return row, nil
}

func recordUnsupportedCompanyHandler(pool *sql.DB) server.ToolHandlerFunc {
	return recordUnsupportedCompanyHandlerWithDeps(recordUnsupportedCompanyPoolExecutor{pool: pool})
}

func recordUnsupportedCompanyHandlerWithDeps(exec recordUnsupportedCompanyExecutor) server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req recordUnsupportedCompanyRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decoding record_unsupported_company arguments: %v", err)), nil
		}

		env := runRecordUnsupportedCompany(ctx, req, exec)
		payload, err := json.Marshal(env)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding record_unsupported_company result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

func runRecordUnsupportedCompany(ctx context.Context, req recordUnsupportedCompanyRequest, exec recordUnsupportedCompanyExecutor) recordUnsupportedCompanyEnvelope {
	name := strings.TrimSpace(req.Name)
	url := strings.TrimSpace(req.URL)
	detectedPlatform := strings.TrimSpace(req.DetectedPlatform)
	reason := strings.TrimSpace(req.Reason)

	if errs := validateRecordUnsupportedCompany(name, url, reason); len(errs) > 0 {
		return recordUnsupportedCompanyFailure(errs)
	}

	row, err := exec.recordUnsupportedCompany(ctx, recordUnsupportedCompanyParams{
		Name:             name,
		URL:              nullStringFrom(url),
		DetectedPlatform: nullStringFrom(detectedPlatform),
		Reason:           reason,
	})
	if err != nil {
		return recordUnsupportedCompanyFailure([]actionError{{
			Path:    "db",
			Code:    codeDBError,
			Message: err.Error(),
		}})
	}

	company := mapUnsupportedCompanyRow(row)
	return recordUnsupportedCompanyEnvelope{
		Ok:                 true,
		UnsupportedCompany: &company,
		Errors:             []actionError{},
	}
}

func validateRecordUnsupportedCompany(name, url, reason string) []actionError {
	var errs []actionError

	if name == "" {
		errs = append(errs, actionError{Path: "name", Code: codeMissingRequired, Message: "name is required"})
	}
	if reason == "" {
		errs = append(errs, actionError{Path: "reason", Code: codeMissingRequired, Message: "reason is required"})
	} else if reason != unsupportedCompanyReasonUnsupportedATS && reason != unsupportedCompanyReasonNoCareers {
		errs = append(errs, actionError{Path: "reason", Code: codeInvalidUnsupportedCompanyReason, Message: "reason must be unsupported_ats or no_careers"})
	}
	if reason == unsupportedCompanyReasonUnsupportedATS && url == "" {
		errs = append(errs, actionError{Path: "url", Code: codeMissingRequired, Message: "url is required when reason is unsupported_ats"})
	}
	if url != "" {
		if err := atsdetect.ValidateURL(url); err != nil {
			errs = append(errs, actionError{Path: "url", Code: codeInvalidURL, Message: fieldURLMessage("url", err)})
		}
	}

	return errs
}

func recordUnsupportedCompanyFailure(errs []actionError) recordUnsupportedCompanyEnvelope {
	return recordUnsupportedCompanyEnvelope{
		Ok:                 false,
		UnsupportedCompany: nil,
		Errors:             errs,
	}
}

func mapUnsupportedCompanyRow(row recordUnsupportedCompanyRow) unsupportedCompanyDTO {
	return unsupportedCompanyDTO{
		ID:               row.ID,
		Name:             row.Name,
		URL:              nullableString(row.URL),
		DetectedPlatform: nullableString(row.DetectedPlatform),
		Reason:           row.Reason,
		FirstSeenAt:      row.FirstSeenAt.UTC().Format(time.RFC3339),
		LastCheckedAt:    row.LastCheckedAt.UTC().Format(time.RFC3339),
	}
}
