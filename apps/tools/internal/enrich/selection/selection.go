// Package selection chooses unclassified job postings for enrichment. It is the
// shared selection core behind both cmd/batch-enrich (which classifies the
// selected postings) and the MCP enrichment_preview tool (which only reports
// what would be selected). The package is HTTP-free and reaches the database
// only through sqlc-generated queries; it never imports cmd/*.
// See: agent-context/lib/developer-guide.md §5.7 (Database access)
package selection

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// Criteria are the inputs that determine which postings selection returns. They
// mirror the batch-enrich flags (--count, --focus, --force) and carry the same
// semantics: Focus is an ILIKE prefilter where `%` and `_` keep their SQL
// wildcard meaning, and Force drops the unclassified guard so already-classified
// postings become eligible.
type Criteria struct {
	Count int
	Focus string
	Force bool
}

// Posting is one selected posting paired with the latest snapshot's title and
// description text and the owning company's display name. CompanyName comes from
// the companies join; companies.name is NOT NULL, so it is never empty by schema.
type Posting struct {
	PostingID       int64
	CompanyID       int64
	CompanyName     string
	Title           string
	DescriptionText string
}

// Querier is the subset of the sqlc db.Queries API selection needs. Declaring it
// here lets tests inject a fake without a live database; *db.Queries satisfies it
// implicitly.
type Querier interface {
	ListUnclassifiedPostings(ctx context.Context, arg db.ListUnclassifiedPostingsParams) ([]db.ListUnclassifiedPostingsRow, error)
	ListUnclassifiedPostingsForced(ctx context.Context, arg db.ListUnclassifiedPostingsForcedParams) ([]db.ListUnclassifiedPostingsForcedRow, error)
	ListClassifiedAmong(ctx context.Context, ids []int64) ([]int64, error)
}

// Select runs the unclassified-postings query (or the forced variant when
// crit.Force is true) and returns the selected postings. When Force is true,
// alreadyClassified holds the subset of selected posting IDs that already have at
// least one classifications row — used downstream to flag duplicate-write
// surprises in re-enrichment. When Force is false the second return is always
// nil, because classified postings are excluded from selection.
func Select(ctx context.Context, pool *sql.DB, crit Criteria) (postings []Posting, alreadyClassified []int64, err error) {
	return SelectWith(ctx, db.New(pool), crit)
}

// SelectWith is the testable core of Select: it takes an explicit Querier so a
// fake can stand in for the sqlc layer. Select wraps it with a *db.Queries built
// from the pool.
func SelectWith(ctx context.Context, q Querier, crit Criteria) (postings []Posting, alreadyClassified []int64, err error) {
	postings, err = selectRows(ctx, q, crit)
	if err != nil {
		return nil, nil, err
	}

	if !crit.Force || len(postings) == 0 {
		return postings, nil, nil
	}

	ids := make([]int64, 0, len(postings))
	for _, p := range postings {
		ids = append(ids, p.PostingID)
	}
	alreadyClassified, err = q.ListClassifiedAmong(ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("checking already-classified postings: %w", err)
	}
	return postings, alreadyClassified, nil
}

// selectRows dispatches to the unclassified or forced variant of the generated
// query and normalises the nullable columns into Posting. The IS NOT NULL filter
// in the SQL guarantees DescriptionText.Valid, so we take its String directly;
// Title remains nullable in the schema and falls back to the empty string when
// absent. CompanyName is non-nullable from the companies join.
func selectRows(ctx context.Context, q Querier, crit Criteria) ([]Posting, error) {
	limit := int32(crit.Count)
	if crit.Force {
		rows, err := q.ListUnclassifiedPostingsForced(ctx, db.ListUnclassifiedPostingsForcedParams{
			Focus:    crit.Focus,
			RowLimit: limit,
		})
		if err != nil {
			return nil, fmt.Errorf("listing unclassified postings (forced): %w", err)
		}
		out := make([]Posting, 0, len(rows))
		for _, r := range rows {
			out = append(out, Posting{
				PostingID:       r.PostingID,
				CompanyID:       r.CompanyID,
				CompanyName:     r.CompanyName,
				Title:           nullStringOr(r.Title, ""),
				DescriptionText: r.DescriptionText.String,
			})
		}
		return out, nil
	}

	rows, err := q.ListUnclassifiedPostings(ctx, db.ListUnclassifiedPostingsParams{
		Focus:    crit.Focus,
		RowLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing unclassified postings: %w", err)
	}
	out := make([]Posting, 0, len(rows))
	for _, r := range rows {
		out = append(out, Posting{
			PostingID:       r.PostingID,
			CompanyID:       r.CompanyID,
			CompanyName:     r.CompanyName,
			Title:           nullStringOr(r.Title, ""),
			DescriptionText: r.DescriptionText.String,
		})
	}
	return out, nil
}

// nullStringOr returns ns.String when valid, otherwise the provided fallback.
func nullStringOr(ns sql.NullString, fallback string) string {
	if ns.Valid {
		return ns.String
	}
	return fallback
}
