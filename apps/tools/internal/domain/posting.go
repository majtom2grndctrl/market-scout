// Package domain holds the shared data types produced by all ingestion sources
// and consumed by the fetcher and future pipeline stages.
// See: agent-context/lib/project.md
package domain

import (
	"encoding/json"
	"time"
)

// Posting is the contract between ATS adapters and the fetcher's DB writer.
// Adapters normalize wire format into this shape; the fetcher writes one row
// per Posting per fetch.
//
// SourceID, SourceURL, and RawData are required. A blank SourceURL would
// collide on the (company_id, source_url) uniqueness key. Empty RawData is a
// contract violation — the fetcher aborts the transaction rather than persist a
// partial board. All three are checked at the fetcher's write boundary;
// adapters must error rather than emit zero values.
//
// Optional fields are pointers; nil means the source did not supply the value
// (persisted as NULL, distinct from an empty string). Slice-typed optional
// fields follow the same contract: nil = NULL, non-nil = array (an empty
// non-nil slice persists as an explicit empty array, distinct from NULL).
type Posting struct {
	SourceID     string
	SourceURL    string
	Title        *string
	LocationText *string
	// LocationTexts is the multi-market location list. nil = source did not
	// supply locations; empty slice = source returned an explicit empty array
	// (rare but distinct); populated = one or more strings. The nil-vs-empty
	// distinction is load-bearing for trend queries on multi-market roles.
	LocationTexts  []string
	Department     *string
	Team           *string
	EmploymentType *string
	WorkplaceType  *string
	PostedAt       *time.Time
	// Raw ATS-reported timestamps. Not used to derive PostedAt.
	SourceFirstPublishedAt *time.Time
	SourceLastModifiedAt   *time.Time
	JobURL                 *string
	DescriptionText        *string
	// Compensation fields are all-or-nothing: adapters set all four
	// (CompensationMin, CompensationMax, CompensationCurrency,
	// CompensationPeriod) together, or none. Min and Max are in whole units
	// of Currency (e.g. dollars, not cents).
	CompensationMin      *int64
	CompensationMax      *int64
	CompensationCurrency *string
	CompensationPeriod   *string
	RawData              json.RawMessage
}
