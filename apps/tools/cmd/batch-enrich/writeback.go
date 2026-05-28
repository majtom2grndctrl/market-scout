// Serial per-posting writeback for batch-enrich. Each enriched posting is
// persisted in its own transaction: get-or-create taxonomy rows, insert the
// classification, then attach join rows. On any failure inside the
// transaction the result's Outcome is downgraded to OutcomeDBFailed so the
// failures.jsonl writer can pick it up — the Classification field is left
// intact for the report.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// writeOneMaxAttempts caps the total attempts (initial + retries) for a
// single posting's writeback transaction. Two retries with short exponential
// backoff is plenty for transient-class errors (serialization conflicts,
// deadlocks, brief network blips); anything that survives three attempts is
// almost certainly a real failure and should surface immediately.
const writeOneMaxAttempts = 3

// writeOneBackoffs is the sleep duration before retries 2 and 3. Index 0 is
// the wait before attempt 2; index 1 is the wait before attempt 3. No sleep
// happens before attempt 1. Kept small because the transaction body is short
// — a multi-second pause would dominate end-to-end wave latency for runs
// that hit even a single transient error.
var writeOneBackoffs = []time.Duration{50 * time.Millisecond, 100 * time.Millisecond}

// WriteBack runs serial writeback for a slice of PostingResults. Only
// results with Outcome == OutcomeEnriched are written; others are returned
// unchanged. Results whose transaction fails have their Outcome rewritten
// to OutcomeDBFailed. The Classification field is never mutated — callers
// downstream still need it for reporting. WriteBack modifies results in place
// and returns the same slice.
func WriteBack(ctx context.Context, results []PostingResult, pool *sql.DB, cfg Config, taxonomy Taxonomy) []PostingResult {
	for i := range results {
		// On context cancellation, stamp any remaining OutcomeEnriched results
		// as OutcomeDBFailed. classifyBatch already marked them enriched, but
		// writeback never persisted them — leaving Outcome=enriched would make
		// the report claim success for postings with no classification row.
		// Because no classifications row was inserted, the selection query's
		// NOT EXISTS guard will reselect these postings on the next run.
		if err := ctx.Err(); err != nil {
			for j := i; j < len(results); j++ {
				if results[j].Outcome == OutcomeEnriched {
					results[j].Outcome = OutcomeDBFailed
					results[j].LastReason = reasonCancelled
				}
			}
			break
		}
		r := &results[i]
		if r.Outcome != OutcomeEnriched || r.Classification == nil {
			continue
		}
		if err := writeOne(ctx, pool, cfg, taxonomy, r); err != nil {
			slog.Error("[batch-enrich] writeback failed",
				"posting_id", r.PostingID,
				"error", err,
			)
			r.Outcome = OutcomeDBFailed
			r.LastReason = err.Error()
		}
	}
	return results
}

// writeOne persists a single enriched posting. Returns an error so the
// caller can downgrade the outcome and log uniformly.
//
// Transient Postgres errors (serialization conflicts, deadlocks) and brief
// network failures are retried up to writeOneMaxAttempts with short
// exponential backoff. Each attempt opens a fresh transaction — a failed
// transaction cannot be reused. Non-transient errors return immediately.
func writeOne(ctx context.Context, pool *sql.DB, cfg Config, taxonomy Taxonomy, r *PostingResult) error {
	return retryTransient(ctx, r.PostingID, func() error {
		return writeOneAttempt(ctx, pool, cfg, taxonomy, r)
	})
}

// retryTransient calls fn up to writeOneMaxAttempts times, sleeping
// writeOneBackoffs[i] before retry i+1. It retries only when isRetryable
// returns true; any other error short-circuits. postingID is used solely
// for log correlation.
func retryTransient(ctx context.Context, postingID int64, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= writeOneMaxAttempts; attempt++ {
		if attempt > 1 {
			// Honor cancellation in the backoff window; otherwise sleep the full
			// interval before reopening the transaction.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(writeOneBackoffs[attempt-2]):
			}
		}
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) {
			return err
		}
		slog.Warn("[batch-enrich] writeback transient failure, retrying",
			"posting_id", postingID,
			"attempt", attempt,
			"error", err,
		)
	}
	return lastErr
}

// writeOneAttempt runs a single writeback transaction. Callers (writeOne)
// invoke it once per attempt; on transient failure the transaction is
// abandoned and writeOne opens a new one.
func writeOneAttempt(ctx context.Context, pool *sql.DB, cfg Config, taxonomy Taxonomy, r *PostingResult) error {
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Rollback is a no-op after Commit; safe to always defer.
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Warn("[batch-enrich] rollback error", "err", err)
		}
	}()

	q := db.New(tx)
	resp := r.Classification.AgentResponse

	roleIDs := make([]int64, 0, len(resp.CanonicalRoles))
	for _, role := range resp.CanonicalRoles {
		roleID, err := q.GetOrCreateCanonicalRole(ctx, db.GetOrCreateCanonicalRoleParams{
			Slug: role.Slug,
			Name: role.Name,
		})
		if err != nil {
			return err
		}
		roleIDs = append(roleIDs, roleID)

		for _, dimSlug := range role.Dimensions {
			// WriteBack receives the wave's current taxonomy snapshot — the same one
			// passed to RunWave for this wave. role_dimensions is a closed set never
			// minted by agents, so the snapshot is sufficient for dimension ID lookups.
			// The two-value form guards against a dimension being deleted between
			// taxonomy load and writeback.
			dim, ok := taxonomy.RoleDimensions[dimSlug]
			if !ok {
				slog.Warn("[batch-enrich] dimension slug missing from taxonomy at writeback time",
					"posting_id", r.PostingID,
					"dim_slug", dimSlug,
				)
				return fmt.Errorf("dimension slug %q not found in taxonomy", dimSlug)
			}
			if err := q.InsertCanonicalRoleDimension(ctx, db.InsertCanonicalRoleDimensionParams{
				CanonicalRoleID: roleID,
				DimensionID:     dim.ID,
			}); err != nil {
				return err
			}
		}
	}

	specIDs := make([]int64, 0, len(resp.Specializations))
	for _, spec := range resp.Specializations {
		specID, err := q.GetOrCreateSpecialization(ctx, db.GetOrCreateSpecializationParams{
			Slug: spec.Slug,
			Name: spec.Name,
		})
		if err != nil {
			return err
		}
		specIDs = append(specIDs, specID)
	}

	skillIDs := make([]int64, 0, len(resp.Skills))
	for _, skill := range resp.Skills {
		skillID, err := q.GetOrCreateSkill(ctx, db.GetOrCreateSkillParams{
			Slug: skill.Slug,
			Name: skill.Name,
		})
		if err != nil {
			return err
		}
		skillIDs = append(skillIDs, skillID)
	}

	seniority := resp.Classification.Seniority

	classID, err := q.InsertClassification(ctx, db.InsertClassificationParams{
		JobPostingID:  r.PostingID,
		Model:         cfg.Model,
		PromptVersion: cfg.PromptVersion,
		Seniority:     seniority,
		Notes:         notesToNullString(resp.Classification.Notes),
	})
	if err != nil {
		return err
	}

	for _, roleID := range roleIDs {
		if err := q.InsertJobPostingRole(ctx, db.InsertJobPostingRoleParams{
			ClassificationID: classID,
			RoleID:           roleID,
		}); err != nil {
			return err
		}
	}
	for _, specID := range specIDs {
		if err := q.InsertJobPostingSpecialization(ctx, db.InsertJobPostingSpecializationParams{
			ClassificationID: classID,
			SpecializationID: specID,
		}); err != nil {
			return err
		}
	}
	for _, skillID := range skillIDs {
		if err := q.InsertJobPostingSkill(ctx, db.InsertJobPostingSkillParams{
			ClassificationID: classID,
			SkillID:          skillID,
		}); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	slog.Info("[batch-enrich] wrote classification",
		"posting_id", r.PostingID,
		"classification_id", classID,
	)
	return nil
}

// isRetryable reports whether err is a transient class we should retry.
//
// Retryable cases:
//   - Postgres serialization_failure (SQLSTATE 40001) — concurrent txns
//     conflicted; rerunning the transaction is the canonical fix.
//   - Postgres deadlock_detected (SQLSTATE 40P01) — same shape; one txn was
//     aborted by the planner to break the cycle.
//   - net.Error with Temporary() == true — kept for parity with stdlib
//     conventions, though most modern net errors no longer set this flag.
//   - io.EOF / io.ErrUnexpectedEOF — broken connection mid-write; pgx may
//     surface these when the server closes the socket between statements.
//
// Anything else (constraint violations, bad SQL, context cancellation,
// programmer errors) is non-retryable: a retry would produce the same
// failure or, worse, mask a real bug.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01":
			return true
		}
		// Any other Postgres error is a definite, non-transient signal.
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Temporary() { //nolint:staticcheck // Temporary() is deprecated but still the lingua franca for transient net errors.
		return true
	}
	return false
}

// notesToNullString maps the agent's notes field to sql.NullString. An
// empty or whitespace-only string becomes NULL; otherwise the raw value is
// preserved (not trimmed) so any operator-visible formatting survives.
func notesToNullString(notes string) sql.NullString {
	if strings.TrimSpace(notes) == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{Valid: true, String: notes}
}
