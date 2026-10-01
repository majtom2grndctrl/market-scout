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
	"errors"
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
	previewMaxCount     = 500

	// previewSampleCap limits how many selected postings the sample carries.
	// selected_count still reflects the full count-limited selection.
	//
	// Note that count bounds work units, not postings: ordinary selection
	// dedups, so a selected unit's siblings ride along and selected_count can
	// exceed count. work_unit_count is the number bounded by previewMaxCount.
	// Exact-ID selection never dedups: every requested posting is its own unit.
	previewSampleCap = 20

	// previewSortOldestFirst and previewSortNewestFirst are the two accepted
	// values for the "sort" param. An empty string (param omitted) resolves to
	// previewSortNewestFirst: oldest-first drains the backlog in arrival order,
	// which keeps the classified set months behind the market.
	previewSortOldestFirst = "oldest_first"
	previewSortNewestFirst = "newest_first"

	// previewMinPerCompany and previewMaxPerCompany bound the
	// max_per_company input. The cap is always on; there is no value that
	// disables it from MCP, because spreading a wave across the watchlist is
	// the coordinator's contract rather than a per-run choice.
	previewMinPerCompany = 1
	previewMaxPerCompany = previewMaxCount
)

// codeInvalidCount is the action error code for a count outside [1, 500].
const codeInvalidCount = "invalid_count"

// codeInvalidSort is the action error code for a sort value other than
// "oldest_first" or "newest_first" (empty string, which means omitted, is
// valid and resolves to the default).
const codeInvalidSort = "invalid_sort"

// codeInvalidMaxPerCompany is the action error code for a max_per_company
// outside [1, 500].
const codeInvalidMaxPerCompany = "invalid_max_per_company"

// codeInvalidPostingIDs is the action error code for a posting_ids list that
// is empty, too long, holds a duplicate or non-positive id, or names postings
// the server cannot return. codeConflictingSelection is the code for
// posting_ids combined with an ordinary selection parameter.
const (
	codeInvalidPostingIDs    = "invalid_posting_ids"
	codeConflictingSelection = "conflicting_selection"
)

// previewRequest is the MCP tool DTO. Count is a pointer so an omitted value
// (default 10) is distinguishable from an explicit 0, which is out of range.
// Sort is a plain string because its own zero value ("") is unambiguous: it
// means "omitted" and resolves to previewSortNewestFirst, never a rejected
// input in its own right. PostingIDs is a pointer so an explicit empty list is
// rejected rather than read as "omitted", which would run an ordinary
// selection in its place.
type previewRequest struct {
	Count *int   `json:"count"`
	Focus string `json:"focus"`
	Force bool   `json:"force"`
	Sort  string `json:"sort"`
	// MaxPerCompany is a pointer for the same reason Count is: an omitted
	// value takes selection.DefaultMaxPerCompany, while an explicit 0 is out
	// of range and must be rejected rather than silently read as "unset".
	MaxPerCompany *int     `json:"max_per_company"`
	PostingIDs    *[]int64 `json:"posting_ids"`
}

// previewEcho mirrors the resolved inputs back to the caller so the agent can
// confirm what the server actually ran with after defaults were applied. In
// exact-ID mode Count is the number of ids, and Sort and MaxPerCompany are
// zero because neither applies.
type previewEcho struct {
	Count         int     `json:"count"`
	Focus         string  `json:"focus"`
	Force         bool    `json:"force"`
	Sort          string  `json:"sort"`
	MaxPerCompany int     `json:"max_per_company"`
	PostingIDs    []int64 `json:"posting_ids,omitempty"`
}

// previewSampleRow is one row of the capped sample: posting id, company id,
// company name, and title.
type previewSampleRow struct {
	PostingID   int64  `json:"posting_id"`
	CompanyID   int64  `json:"company_id"`
	CompanyName string `json:"company_name"`
	Title       string `json:"title"`
}

// previewPostingRow is one ordered selected posting. It is the coordinator's
// dispatch cohort and comes directly from the shared selection result; Sample
// remains separately capped for lightweight inspection callers.
type previewPostingRow struct {
	PostingID   int64  `json:"posting_id"`
	CompanyID   int64  `json:"company_id"`
	CompanyName string `json:"company_name"`
	Title       string `json:"title"`
	// DedupKey groups postings that describe the same job; IsRepresentative
	// marks the one whose text a worker reads. A coordinator dispatches whole
	// units, classifies the representative once, and saves that result to every
	// posting sharing the key.
	DedupKey         string `json:"dedup_key"`
	IsRepresentative bool   `json:"is_representative"`
}

// previewEnvelope is the JSON the tool returns. An invalid count sets Ok=false
// with an errors[] entry rather than an MCP transport error. SelectedCount is the
// number of rows selected after the count limit; SampleCount is the number of
// rows in Sample; AlreadyClassifiedCount is always present. It is 0 for an
// ordinary selection without force, and counts already-classified postings in
// an exact-ID repair cohort.
type previewEnvelope struct {
	Ok                     bool                `json:"ok"`
	Input                  previewEcho         `json:"input"`
	SelectedCount          int                 `json:"selected_count"`
	WorkUnitCount          int                 `json:"work_unit_count"`
	SampleCount            int                 `json:"sample_count"`
	AlreadyClassifiedCount int                 `json:"already_classified_count"`
	Sample                 []previewSampleRow  `json:"sample"`
	Postings               []previewPostingRow `json:"postings"`
	Errors                 []actionError       `json:"errors"`
}

// previewSelector is the selection seam the handler depends on. The production
// implementation calls the shared selection core against the read-only pool;
// tests inject a fake to assert validation, defaults, and response mapping
// without a database.
type previewSelector interface {
	Select(ctx context.Context, crit selection.Criteria) ([]selection.Posting, []int64, error)
	SelectIDs(ctx context.Context, ids []int64) ([]selection.Posting, []int64, error)
}

// poolSelector adapts selection.Select to the previewSelector seam, binding the
// read-only pool.
type poolSelector struct {
	pool *sql.DB
}

func (s poolSelector) Select(ctx context.Context, crit selection.Criteria) ([]selection.Posting, []int64, error) {
	return selection.Select(ctx, s.pool, crit)
}

func (s poolSelector) SelectIDs(ctx context.Context, ids []int64) ([]selection.Posting, []int64, error) {
	return selection.SelectIDs(ctx, s.pool, ids)
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

// runEnrichmentPreview resolves defaults, validates count and max_per_company
// against their 1..500 bounds and sort against its two accepted values, then
// runs the shared selection and maps the result into the preview envelope.
// When posting_ids is present it instead validates the list and runs exact-ID
// selection; the ordinary parameters must then be absent. An invalid input
// returns an ok=false envelope; it never returns an MCP transport error.
func runEnrichmentPreview(ctx context.Context, req previewRequest, sel previewSelector) previewEnvelope {
	count := previewDefaultCount
	if req.Count != nil {
		count = *req.Count
	}
	sort := req.Sort
	if sort == "" {
		sort = previewSortNewestFirst
	}
	maxPerCompany := selection.DefaultMaxPerCompany
	if req.MaxPerCompany != nil {
		maxPerCompany = *req.MaxPerCompany
	}
	echo := previewEcho{
		Count:         count,
		Focus:         req.Focus,
		Force:         req.Force,
		Sort:          sort,
		MaxPerCompany: maxPerCompany,
	}

	if req.PostingIDs != nil {
		ids := *req.PostingIDs
		echo = previewEcho{Count: len(ids), Focus: req.Focus, Force: req.Force, PostingIDs: ids}
		if len(ids) == 0 {
			return previewFailure(echo, "posting_ids", codeInvalidPostingIDs,
				"posting_ids must contain at least one id; omit it for ordinary selection")
		}
		if req.Count != nil || req.Focus != "" || req.Force || req.Sort != "" || req.MaxPerCompany != nil {
			return previewFailure(echo, "posting_ids", codeConflictingSelection,
				"posting_ids is mutually exclusive with count, focus, force, sort, and max_per_company")
		}
		seen := make(map[int64]struct{}, len(ids))
		for i, id := range ids {
			if id <= 0 {
				return previewFailure(echo, fmt.Sprintf("posting_ids[%d]", i), codeInvalidPostingIDs,
					"posting ids must be positive integers")
			}
			if _, duplicate := seen[id]; duplicate {
				return previewFailure(echo, fmt.Sprintf("posting_ids[%d]", i), codeInvalidPostingIDs,
					fmt.Sprintf("posting id %d appears more than once", id))
			}
			seen[id] = struct{}{}
		}
		if len(ids) > previewMaxCount {
			return previewFailure(echo, "posting_ids", codeInvalidPostingIDs,
				fmt.Sprintf("posting_ids must contain at most %d ids", previewMaxCount))
		}
		postings, alreadyClassified, err := sel.SelectIDs(ctx, ids)
		var missing *selection.MissingPostingsError
		if errors.As(err, &missing) {
			return previewFailure(echo, "posting_ids", codeInvalidPostingIDs, err.Error())
		}
		if err != nil {
			return previewFailure(echo, "db", codeDBError, err.Error())
		}
		return buildPreviewEnvelope(echo, postings, alreadyClassified)
	}

	if count < previewMinCount || count > previewMaxCount {
		return previewFailure(echo, "count", codeInvalidCount,
			fmt.Sprintf("count must be between %d and %d", previewMinCount, previewMaxCount))
	}

	if sort != previewSortOldestFirst && sort != previewSortNewestFirst {
		return previewFailure(echo, "sort", codeInvalidSort,
			fmt.Sprintf("sort must be %q or %q", previewSortOldestFirst, previewSortNewestFirst))
	}

	if maxPerCompany < previewMinPerCompany || maxPerCompany > previewMaxPerCompany {
		return previewFailure(echo, "max_per_company", codeInvalidMaxPerCompany,
			fmt.Sprintf("max_per_company must be between %d and %d", previewMinPerCompany, previewMaxPerCompany))
	}

	critSort := selection.SortOldestFirst
	if sort == previewSortNewestFirst {
		critSort = selection.SortNewestFirst
	}

	// Dedup is always on for preview. The direct-MCP coordinator dispatches the
	// list this tool returns, and its plan forbids it from reimplementing
	// selection SQL — so grouping has to happen here or not at all.
	postings, alreadyClassified, err := sel.Select(ctx, selection.Criteria{
		Count:         count,
		Focus:         req.Focus,
		Force:         req.Force,
		Sort:          critSort,
		Dedup:         true,
		MaxPerCompany: maxPerCompany,
	})
	if err != nil {
		return previewFailure(echo, "db", codeDBError,
			err.Error())
	}

	return buildPreviewEnvelope(echo, postings, alreadyClassified)
}

// previewFailure is the ok=false envelope for one invalid input or fault.
func previewFailure(input previewEcho, path, code, message string) previewEnvelope {
	return previewEnvelope{Ok: false, Input: input, Errors: []actionError{{Path: path, Code: code, Message: message}}, Sample: []previewSampleRow{}, Postings: []previewPostingRow{}}
}

// buildPreviewEnvelope maps a selection result into the ok=true envelope.
func buildPreviewEnvelope(echo previewEcho, postings []selection.Posting, alreadyClassified []int64) previewEnvelope {
	sample := make([]previewSampleRow, 0, min(len(postings), previewSampleCap))
	selected := make([]previewPostingRow, 0, len(postings))
	workUnits := 0
	for _, p := range postings {
		if p.IsRepresentative {
			workUnits++
		}
		selected = append(selected, previewPostingRow{
			PostingID: p.PostingID, CompanyID: p.CompanyID, CompanyName: p.CompanyName, Title: p.Title,
			DedupKey: p.DedupKey, IsRepresentative: p.IsRepresentative,
		})
		if len(sample) >= previewSampleCap {
			continue
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
		WorkUnitCount:          workUnits,
		SampleCount:            len(sample),
		AlreadyClassifiedCount: len(alreadyClassified),
		Sample:                 sample,
		Postings:               selected,
		Errors:                 []actionError{},
	}
}
