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
	"math"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// Sort selects the ordering Select returns postings in.
//
// The zero value is SortDefault, not a direction. Go's "make the zero value
// useful" rule says Criteria{} should already do the right thing, and it does:
// SortDefault resolves to SortNewestFirst. But naming the zero value after a
// direction — as this package did while SortOldestFirst was iota's first
// constant — makes the default a property of that constant rather than of the
// package, so flipping the policy silently changes what every existing
// Criteria{} means with nothing for the compiler to catch. SortDefault is an
// indirection instead: it means "whatever selection's current policy is", the
// two direction constants keep stable meanings forever, and a future policy
// change is one line in resolve().
type Sort int

const (
	// SortDefault is the zero value and resolves to SortNewestFirst.
	SortDefault Sort = iota
	// SortNewestFirst orders by first_seen_at descending — the recency-first
	// policy. Oldest-first drains the backlog in arrival order, which
	// structurally guarantees the classified set lags the market: on
	// 2026-09-21 only 4 of 1,946 classified postings had been first seen in
	// the preceding three weeks, against 1,108 of 2,712 unclassified ones.
	SortNewestFirst
	// SortOldestFirst orders by first_seen_at ascending. Still reachable, for
	// a deliberate backlog drain.
	SortOldestFirst
)

// resolve maps the zero value onto the package's current default direction.
func (s Sort) resolve() Sort {
	if s == SortDefault {
		return SortNewestFirst
	}
	return s
}

const (
	// DefaultMaxPerCompany is the per-company ceiling applied when
	// Criteria.MaxPerCompany is zero. It bounds how much of one wave a single
	// company can occupy so the classified set is broader than the corpus,
	// which is ~43% OpenAI/Stripe/Anthropic. Five is low on purpose: this is
	// under-sampling the giants, not sampling them proportionally.
	DefaultMaxPerCompany = 5
	// NoCompanyCap disables the per-company ceiling. Any negative value does,
	// but callers should say so with this constant.
	NoCompanyCap = -1
)

// Criteria are the inputs that determine which postings selection returns. They
// mirror the batch-enrich flags (--count, --focus, --force, --sort,
// --max-per-company) and carry the same semantics: Focus is an ILIKE prefilter
// where `%` and `_` keep their SQL wildcard meaning, and Force drops the
// unclassified guard so already-classified postings become eligible. Sort
// chooses the first_seen_at direction; its zero value resolves to newest-first.
//
// MaxPerCompany caps how many work units one company contributes to a wave, and
// the query orders across companies before recency so a wave spreads over the
// watchlist. Zero means DefaultMaxPerCompany; NoCompanyCap (or any negative
// value) removes the ceiling. The cap counts work units, matching Count, so a
// capped company can still return more postings than the cap when its selected
// units have siblings.
//
// Dedup collapses postings describing the same job into one work unit. It
// changes what Count bounds: work units rather than postings, with every member
// of a selected unit returned even past the limit, so callers must expect
// len(postings) >= Count. The zero value is false, which preserves the
// row-per-posting semantics cmd/batch-enrich depends on; enrichment_preview sets
// it so the direct-MCP coordinator never has to reimplement grouping.
type Criteria struct {
	Count         int
	Focus         string
	Force         bool
	Sort          Sort
	Dedup         bool
	MaxPerCompany int
}

// Posting is one selected posting paired with the latest snapshot's title and
// description text and the owning company's display name. CompanyName comes from
// the companies join; companies.name is NOT NULL, so it is never empty by schema.
// DedupKey identifies the work unit a posting belongs to; postings sharing one
// describe the same job. A unit is a connected component over two company-scoped
// edges — identical description text, and a shared requisition key under a
// matching normalized title — so the rule is transitive and lives entirely in
// internal/db/queries/enrich.sql. IsRepresentative marks the single member whose
// text a classifier should actually read — the lowest posting id in the unit.
// With Criteria.Dedup false no edges are built, so every posting is its own unit
// and IsRepresentative is always true.
type Posting struct {
	PostingID        int64
	CompanyID        int64
	CompanyName      string
	Title            string
	DescriptionText  string
	DedupKey         string
	IsRepresentative bool
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
	newestFirst := crit.Sort.resolve() == SortNewestFirst
	maxPerCompany := resolveMaxPerCompany(crit.MaxPerCompany)
	if crit.Force {
		rows, err := q.ListUnclassifiedPostingsForced(ctx, db.ListUnclassifiedPostingsForcedParams{
			Focus:         crit.Focus,
			NewestFirst:   newestFirst,
			RowLimit:      limit,
			Dedup:         crit.Dedup,
			MaxPerCompany: maxPerCompany,
		})
		if err != nil {
			return nil, fmt.Errorf("listing unclassified postings (forced): %w", err)
		}
		out := make([]Posting, 0, len(rows))
		for _, r := range rows {
			out = append(out, Posting{
				PostingID:        r.PostingID,
				CompanyID:        r.CompanyID,
				CompanyName:      r.CompanyName,
				Title:            nullStringOr(r.Title, ""),
				DescriptionText:  r.DescriptionText.String,
				DedupKey:         r.DedupKey,
				IsRepresentative: r.IsRepresentative,
			})
		}
		return out, nil
	}

	rows, err := q.ListUnclassifiedPostings(ctx, db.ListUnclassifiedPostingsParams{
		Focus:         crit.Focus,
		NewestFirst:   newestFirst,
		RowLimit:      limit,
		Dedup:         crit.Dedup,
		MaxPerCompany: maxPerCompany,
	})
	if err != nil {
		return nil, fmt.Errorf("listing unclassified postings: %w", err)
	}
	out := make([]Posting, 0, len(rows))
	for _, r := range rows {
		out = append(out, Posting{
			PostingID:        r.PostingID,
			CompanyID:        r.CompanyID,
			CompanyName:      r.CompanyName,
			Title:            nullStringOr(r.Title, ""),
			DescriptionText:  r.DescriptionText.String,
			DedupKey:         r.DedupKey,
			IsRepresentative: r.IsRepresentative,
		})
	}
	return out, nil
}

// resolveMaxPerCompany turns the Criteria field into the query parameter. Zero
// means "unset" and takes the package default; negative means no cap, which the
// query spells as a non-positive @max_per_company. The clamp keeps an absurd
// caller-supplied value from wrapping when it narrows to the int32 the
// generated parameter takes.
func resolveMaxPerCompany(n int) int32 {
	switch {
	case n == 0:
		return DefaultMaxPerCompany
	case n < 0:
		return 0
	case n > math.MaxInt32:
		return math.MaxInt32
	default:
		return int32(n)
	}
}

// nullStringOr returns ns.String when valid, otherwise the provided fallback.
func nullStringOr(ns sql.NullString, fallback string) string {
	if ns.Valid {
		return ns.String
	}
	return fallback
}
