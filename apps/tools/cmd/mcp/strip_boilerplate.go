// strip_boilerplate exposes the company-scoped preprocessing step that workers
// use before classification. It binds only the read-only pool and returns text
// for explicitly selected posting ids; it never exposes a general database DSN.
// See: agent-context/plans/done/codex-native-batch-enrichment/index.md
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/boilerplate"
)

const (
	stripBoilerplateMinSelected = 1
	stripBoilerplateMaxSelected = 100
	codeInvalidSelectedIDs      = "invalid_selected_ids"
)

type stripBoilerplateRequest struct {
	CompanyID   int64   `json:"company_id"`
	SelectedIDs []int64 `json:"selected_ids"`
}

type stripBoilerplateRow struct {
	PostingID   int64  `json:"posting_id"`
	CleanedText string `json:"cleaned_text"`
}

type stripBoilerplateEnvelope struct {
	Ok        bool                  `json:"ok"`
	CompanyID int64                 `json:"company_id"`
	Postings  []stripBoilerplateRow `json:"postings"`
	Errors    []actionError         `json:"errors"`
}

// stripBoilerplateHandler binds the tool to the read-only pool. The shared
// loader verifies every selected id before returning any cleaned description.
func stripBoilerplateHandler(pool *sql.DB) server.ToolHandlerFunc {
	return stripBoilerplateHandlerWithLoader(boilerplate.NewDBLoader(db.New(pool)))
}

func stripBoilerplateHandlerWithLoader(loader boilerplate.Loader) server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req stripBoilerplateRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decoding strip_boilerplate arguments: %v", err)), nil
		}

		env := runStripBoilerplate(ctx, req, loader)
		payload, err := json.Marshal(env)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding strip_boilerplate result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

func runStripBoilerplate(ctx context.Context, req stripBoilerplateRequest, loader boilerplate.Loader) stripBoilerplateEnvelope {
	if req.CompanyID == 0 {
		return stripBoilerplateFailure(req.CompanyID, []actionError{{
			Path: "company_id", Code: codeMissingRequired, Message: "company_id is required and must be non-zero",
		}})
	}
	if len(req.SelectedIDs) < stripBoilerplateMinSelected || len(req.SelectedIDs) > stripBoilerplateMaxSelected {
		return stripBoilerplateFailure(req.CompanyID, []actionError{{
			Path: "selected_ids", Code: codeInvalidSelectedIDs,
			Message: fmt.Sprintf("selected_ids must contain between %d and %d ids", stripBoilerplateMinSelected, stripBoilerplateMaxSelected),
		}})
	}

	postings, err := boilerplate.CleanSelected(ctx, loader, req.CompanyID, req.SelectedIDs)
	if err != nil {
		var invalid *boilerplate.InvalidSelectionError
		if errors.As(err, &invalid) {
			errs := make([]actionError, 0, len(invalid.Issues))
			for _, issue := range invalid.Issues {
				errs = append(errs, actionError{
					Path: fmt.Sprintf("selected_ids[%d]", issue.Index),
					Code: string(issue.Code), Message: issue.Message,
				})
			}
			return stripBoilerplateFailure(req.CompanyID, errs)
		}
		return stripBoilerplateFailure(req.CompanyID, []actionError{{
			Path: "db", Code: codeDBError, Message: err.Error(),
		}})
	}

	result := make([]stripBoilerplateRow, 0, len(postings))
	for _, posting := range postings {
		result = append(result, stripBoilerplateRow{PostingID: posting.PostingID, CleanedText: posting.DescriptionText})
	}
	return stripBoilerplateEnvelope{
		Ok: true, CompanyID: req.CompanyID, Postings: result, Errors: []actionError{},
	}
}

func stripBoilerplateFailure(companyID int64, errs []actionError) stripBoilerplateEnvelope {
	return stripBoilerplateEnvelope{
		Ok: false, CompanyID: companyID, Postings: []stripBoilerplateRow{}, Errors: errs,
	}
}
