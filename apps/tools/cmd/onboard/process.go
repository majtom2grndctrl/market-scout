package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// recencyDays is the dedup recency window from agent-context/lib/watchlist.md
// §Dedup. A 30-day window means "a company we've fetched at least once in
// the last month is a duplicate".
const recencyDays = 30

// dedupQuerier is the subset of the sqlc-generated *db.Queries surface the
// processor needs. Declared here (the consumer) so process_test.go can stub
// out the DB without standing up Postgres.
type dedupQuerier interface {
	FindCompanyDedupStatus(ctx context.Context, arg db.FindCompanyDedupStatusParams) (db.FindCompanyDedupStatusRow, error)
}

// processor carries the per-run wiring shared by every record. One
// processor is constructed at startup and reused for all records — adapter
// constructors and the HTTP client are reusable across companies.
type processor struct {
	queries    dedupQuerier
	httpClient *http.Client
	runID      string
	now        func() time.Time
	seedGroup  string
}

// outcome is the result of processing one record. status is set when the
// record reaches a terminal state; verified is true when the record was
// stamped verified. seedRow is populated only on a verified outcome and
// only when the verified ATS is supported (always, since unsupported ATS
// is a terminal status that returns before stamping verified).
type outcome struct {
	verified bool
	status   string
	seedRow  *seedRow
	// preconditionMissing is set when the record cannot be processed because
	// it lacks required inputs (url, careers_url). The runner aggregates
	// these into a single exit-2 path. The record's on-disk fields are not
	// mutated in this case. Reserved for genuine annotator omissions —
	// transport/DB errors are returned via the second return value of
	// processRecord and drive a generic exit-1, not exit-2.
	preconditionMissing string
}

// processRecord runs the per-record state machine. The Record is mutated in
// place when a terminal status or verified_at is reached; the caller
// persists the result via writeSidecar at the end of the run. Returns a
// non-nil error only on transport/DB failures not attributable to the
// record's content (spec §Exit semantics: generic exit 1). Per-record
// annotator omissions surface via outcome.preconditionMissing instead.
// See: agent-context/lib/watchlist.md §Research file annotation.
func (p *processor) processRecord(ctx context.Context, rec *Record) (outcome, error) {
	// Step 1+2: skip if already terminal or verified. No mutation, no probe.
	if rec.IsTerminal() {
		return outcome{status: *rec.Status}, nil
	}
	if rec.IsVerified() {
		return outcome{verified: true}, nil
	}

	// Precondition: url must be set per spec §Per-record behavior. The
	// homepage probe was deliberately removed from the spec; presence of
	// url is the annotator's confirmation that the company is live.
	if rec.URL == nil || *rec.URL == "" {
		return outcome{preconditionMissing: "url"}, nil
	}

	// careers_url is the annotator's hook for ATS detection. Without one
	// AND without (ats, board_token) we cannot probe at all — treat as a
	// precondition failure, not a no-careers terminal. no-careers is the
	// outcome of an actual probe failure; the annotator can set it
	// explicitly if they've already determined the company has no careers
	// page.
	if (rec.CareersURL == nil || *rec.CareersURL == "") && rec.ATS == nil {
		return outcome{preconditionMissing: "careers_url"}, nil
	}

	// Step 3: DB dedup check (only meaningful when ats+board_token are
	// already set). Records with neither set skip this step and run dedup
	// after ATS auto-detection below — the dedup query needs the pair to
	// match against.
	if rec.ATS != nil && rec.BoardToken != nil {
		res, err := p.dedupAndStamp(ctx, rec)
		if err != nil {
			return outcome{}, err
		}
		if res != nil {
			return *res, nil
		}
	}

	// Step 4: careers URL probe. The spec treats this as advisory when ats
	// is already set — the ATS probe in step 5 is the truth signal — but a
	// non-2xx careers URL with no ats at all is the no-careers gate, after
	// we attempt detection on the URL pattern itself.
	if rec.CareersURL != nil && *rec.CareersURL != "" {
		careersErr := probeCareersURL(ctx, p.httpClient, *rec.CareersURL)
		if careersErr != nil {
			// A failed careers probe with no ats yet does NOT immediately
			// mean no-careers. The URL itself may still match a known ATS
			// pattern (e.g. a 404 on boards.greenhouse.io/foo still implies
			// greenhouse; the ATS probe will then likely return
			// invalid-token). Only if detection also fails to recognize the
			// URL pattern do we stamp no-careers.
			if rec.ATS == nil {
				det := detectATS(*rec.CareersURL)
				if !det.recognized {
					rec.Status = strPtr("no-careers")
					return outcome{status: "no-careers"}, nil
				}
				rec.ATS = strPtr(det.ats)
				rec.BoardToken = strPtr(det.boardToken)
				// Auto-detected ats/board_token requires re-running dedup;
				// the initial dedup check ran with both nil and skipped.
				res, err := p.dedupAndStamp(ctx, rec)
				if err != nil {
					return outcome{}, err
				}
				if res != nil {
					return *res, nil
				}
			} else {
				slog.Warn("[onboard] careers_url probe failed (advisory)", "rank", rec.Rank, "name", rec.Name, "error", careersErr)
			}
		} else if rec.ATS == nil {
			// Careers probe succeeded; derive ats/board_token from the URL.
			// ATS detection consults the careers URL, not the company
			// homepage — homepage URLs rarely encode ATS slugs.
			det := detectATS(*rec.CareersURL)
			if !det.recognized {
				rec.Status = strPtr("unsupported-ats")
				return outcome{status: "unsupported-ats"}, nil
			}
			rec.ATS = strPtr(det.ats)
			rec.BoardToken = strPtr(det.boardToken)
			res, err := p.dedupAndStamp(ctx, rec)
			if err != nil {
				return outcome{}, err
			}
			if res != nil {
				return *res, nil
			}
		}
	}

	// At this point a record without ats/board_token cannot be probed.
	// Annotator's responsibility — treat as a precondition failure.
	if rec.ATS == nil || rec.BoardToken == nil || *rec.ATS == "" || *rec.BoardToken == "" {
		return outcome{preconditionMissing: "ats-or-board-token"}, nil
	}

	// Step 5: ATS probe. FetchPostings is the truth signal per the spec.
	adapter, ok := adapterFor(*rec.ATS, p.httpClient)
	if !ok {
		rec.Status = strPtr("unsupported-ats")
		return outcome{status: "unsupported-ats"}, nil
	}
	if _, err := adapter.FetchPostings(ctx, *rec.BoardToken); err != nil {
		slog.Info("[onboard] ATS probe failed", "rank", rec.Rank, "ats", *rec.ATS, "board_token", *rec.BoardToken, "error", err)
		rec.Status = strPtr("invalid-token")
		return outcome{status: "invalid-token"}, nil
	}

	// Step 6: stamp verified. RFC3339 UTC matches the schema's stated format.
	ts := p.now().UTC().Format(time.RFC3339)
	rec.VerifiedAt = strPtr(ts)
	rec.VerifiedRunID = strPtr(p.runID)

	// Step 7: emit a seed row. Industry is taken from the source sub-object
	// verbatim when present — the spec marks industry normalization as out
	// of scope; whatever the research list said is the best signal we have
	// for the initial seed.
	industry := ""
	if rec.Source.Industry != nil {
		industry = *rec.Source.Industry
	}
	return outcome{
		verified: true,
		seedRow: &seedRow{
			Name:       rec.Name,
			ATS:        *rec.ATS,
			BoardToken: *rec.BoardToken,
			Industry:   industry,
			Group:      p.seedGroup,
		},
	}, nil
}

// dedupAndStamp runs the dedup query for the record's (ats, board_token)
// pair, stamps a terminal status on the record if appropriate, and returns
// either a non-nil outcome (terminal) or nil (no match — caller continues).
// Errors are propagated upward as transport/DB failures, not precondition
// gaps. Caller must have validated rec.ATS and rec.BoardToken non-nil.
func (p *processor) dedupAndStamp(ctx context.Context, rec *Record) (*outcome, error) {
	dup, err := p.checkDedup(ctx, *rec.ATS, *rec.BoardToken)
	if err != nil {
		slog.Error("[onboard] dedup query failed", "rank", rec.Rank, "name", rec.Name, "error", err)
		return nil, fmt.Errorf("dedup query for rank %d (%s): %w", rec.Rank, rec.Name, err)
	}
	switch dup {
	case dedupFreshDuplicate:
		rec.Status = strPtr("duplicate")
		return &outcome{status: "duplicate"}, nil
	case dedupStale:
		rec.Status = strPtr("stale-needs-merge")
		return &outcome{status: "stale-needs-merge"}, nil
	case dedupNoMatch:
		return nil, nil
	}
	return nil, nil
}

// dedupResult is the trichotomy from agent-context/lib/watchlist.md §Dedup.
type dedupResult int

const (
	dedupNoMatch dedupResult = iota
	dedupFreshDuplicate
	dedupStale
)

func (p *processor) checkDedup(ctx context.Context, ats, boardToken string) (dedupResult, error) {
	row, err := p.queries.FindCompanyDedupStatus(ctx, db.FindCompanyDedupStatusParams{
		RecencyDays: recencyDays,
		Ats:         ats,
		BoardToken:  boardToken,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dedupNoMatch, nil
		}
		return dedupNoMatch, fmt.Errorf("dedup query for (%s, %s): %w", ats, boardToken, err)
	}
	if row.HasRecentSnapshot {
		return dedupFreshDuplicate, nil
	}
	return dedupStale, nil
}
