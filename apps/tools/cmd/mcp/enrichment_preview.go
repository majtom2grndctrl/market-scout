// enrichment_preview is a read-only MCP tool: it reports what batch-enrich would
// select without spawning agents or writing anything. It reuses the shared
// selection core (internal/enrich/selection) so its rules match batch-enrich
// exactly, and binds the read-only pool — the RO role already has the table reads
// selection needs. The agent sends only typed parameters; the server never relays
// SQL.
// See: agent-context/lib/developer-guide.md §6.2 (enrichment inspection)
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/selection"
)

const (
	// previewDefaultCount, previewMinCount, and previewMaxCount bound the count
	// input. MCP enforces these before calling shared selection logic so an
	// out-of-range request is a structured action error, not an unbounded query.
	previewDefaultCount = 10
	previewMinCount     = 1
	previewMaxCount     = 100

	// previewSampleCap limits how many selected postings the sample carries.
	// selected_count still reflects the full count-limited selection.
	previewSampleCap = 20

	// previewSortOldestFirst and previewSortNewestFirst are the two accepted
	// values for the "sort" param. An empty string (param omitted) resolves to
	// previewSortOldestFirst, matching the pre-existing default behavior.
	previewSortOldestFirst = "oldest_first"
	previewSortNewestFirst = "newest_first"
)

// codeInvalidCount is the action error code for a count outside [1, 100].
const codeInvalidCount = "invalid_count"

// codeInvalidSort is the action error code for a sort value other than
// "oldest_first" or "newest_first" (empty string, which means omitted, is
// valid and resolves to the default).
const codeInvalidSort = "invalid_sort"

// previewRequest is the MCP tool DTO. Count is a pointer so an omitted value
// (default 10) is distinguishable from an explicit 0, which is out of range.
// Sort is a plain string because its own zero value ("") is unambiguous: it
// means "omitted" and resolves to previewSortOldestFirst, never a rejected
// input in its own right.
type previewRequest struct {
	Count *int   `json:"count"`
	Focus string `json:"focus"`
	Force bool   `json:"force"`
	Sort  string `json:"sort"`
}

// previewEcho mirrors the resolved inputs back to the caller so the agent can
// confirm what the server actually ran with after defaults were applied.
type previewEcho struct {
	Count int    `json:"count"`
	Focus string `json:"focus"`
	Force bool   `json:"force"`
	Sort  string `json:"sort"`
}

// previewSampleRow is one row of the capped sample: posting id, company id,
// company name, and title.
type previewSampleRow struct {
	PostingID   int64  `json:"posting_id"`
	CompanyID   int64  `json:"company_id"`
	CompanyName string `json:"company_name"`
	Title       string `json:"title"`
}

// previewEnvelope is the JSON the tool returns. An invalid count sets Ok=false
// with an errors[] entry rather than an MCP transport error. SelectedCount is the
// number of rows selected after the count limit; SampleCount is the number of
// rows in Sample; AlreadyClassifiedCount is always present (0 when force=false).
type previewEnvelope struct {
	Ok                     bool               `json:"ok"`
	Input                  previewEcho        `json:"input"`
	SelectedCount          int                `json:"selected_count"`
	SampleCount            int                `json:"sample_count"`
	AlreadyClassifiedCount int                `json:"already_classified_count"`
	Sample                 []previewSampleRow `json:"sample"`
	Errors                 []actionError      `json:"errors"`
}

// previewSelector is the selection seam the handler depends on. The production
// implementation calls the shared selection core against the read-only pool;
// tests inject a fake to assert validation, defaults, and response mapping
// without a database.
type previewSelector interface {
	Select(ctx context.Context, crit selection.Criteria) ([]selection.Posting, []int64, error)
}

// poolSelector adapts selection.Select to the previewSelector seam, binding the
// read-only pool.
type poolSelector struct {
	pool *sql.DB
}

func (s poolSelector) Select(ctx context.Context, crit selection.Criteria) ([]selection.Posting, []int64, error) {
	return selection.Select(ctx, s.pool, crit)
}

// enrichmentPreviewHandler wires the tool to the read-only pool. Preview never
// writes, so it deliberately receives the read-only handle, never the action pool.
func enrichmentPreviewHandler(pool *sql.DB) server.ToolHandlerFunc {
	return enrichmentPreviewHandlerWithDeps(poolSelector{pool: pool})
}

// enrichmentPreviewHandlerWithDeps is the testable handler body. Tests inject a
// fake selector; production wiring passes the read-only pool via
// enrichmentPreviewHandler.
func enrichmentPreviewHandlerWithDeps(sel previewSelector) server.ToolHandlerFunc {
	return func(ctx context.Context, mcpReq mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var req previewRequest
		if err := mcpReq.BindArguments(&req); err != nil {
			// A request the server cannot decode is a genuinely malformed tool
			// call — the one case that warrants an MCP transport error.
			return mcp.NewToolResultError(fmt.Sprintf("decoding enrichment_preview arguments: %v", err)), nil
		}

		env := runEnrichmentPreview(ctx, req, sel)
		payload, err := json.Marshal(env)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("encoding enrichment_preview result: %v", err)), nil
		}
		return mcp.NewToolResultText(string(payload)), nil
	}
}

// runEnrichmentPreview resolves defaults, validates count against the 1..100
// bound and sort against its two accepted values, then runs the shared
// selection and maps the result into the preview envelope. An invalid count or
// sort returns an ok=false envelope; it never returns an MCP transport error.
func runEnrichmentPreview(ctx context.Context, req previewRequest, sel previewSelector) previewEnvelope {
	count := previewDefaultCount
	if req.Count != nil {
		count = *req.Count
	}
	sort := req.Sort
	if sort == "" {
		sort = previewSortOldestFirst
	}
	echo := previewEcho{Count: count, Focus: req.Focus, Force: req.Force, Sort: sort}

	if count < previewMinCount || count > previewMaxCount {
		return previewEnvelope{
			Ok:    false,
			Input: echo,
			Errors: []actionError{{
				Path:    "count",
				Code:    codeInvalidCount,
				Message: fmt.Sprintf("count must be between %d and %d", previewMinCount, previewMaxCount),
			}},
			Sample: []previewSampleRow{},
		}
	}

	if sort != previewSortOldestFirst && sort != previewSortNewestFirst {
		return previewEnvelope{
			Ok:    false,
			Input: echo,
			Errors: []actionError{{
				Path:    "sort",
				Code:    codeInvalidSort,
				Message: fmt.Sprintf("sort must be %q or %q", previewSortOldestFirst, previewSortNewestFirst),
			}},
			Sample: []previewSampleRow{},
		}
	}

	critSort := selection.SortOldestFirst
	if sort == previewSortNewestFirst {
		critSort = selection.SortNewestFirst
	}

	postings, alreadyClassified, err := sel.Select(ctx, selection.Criteria{
		Count: count,
		Focus: req.Focus,
		Force: req.Force,
		Sort:  critSort,
	})
	if err != nil {
		return previewEnvelope{
			Ok:    false,
			Input: echo,
			Errors: []actionError{{
				Path:    "db",
				Code:    codeDBError,
				Message: err.Error(),
			}},
			Sample: []previewSampleRow{},
		}
	}

	sample := make([]previewSampleRow, 0, min(len(postings), previewSampleCap))
	for _, p := range postings {
		if len(sample) >= previewSampleCap {
			break
		}
		sample = append(sample, previewSampleRow{
			PostingID:   p.PostingID,
			CompanyID:   p.CompanyID,
			CompanyName: p.CompanyName,
			Title:       p.Title,
		})
	}

	return previewEnvelope{
		Ok:                     true,
		Input:                  echo,
		SelectedCount:          len(postings),
		SampleCount:            len(sample),
		AlreadyClassifiedCount: len(alreadyClassified),
		Sample:                 sample,
		Errors:                 []actionError{},
	}
}
