// Command onboard verifies sidecar JSONL records of candidate companies,
// stamps verified rows, and appends seed inserts to internal/db/seeds/companies.sql.
// See: agent-context/lib/watchlist.md
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/internal/db"
	"github.com/oklog/ulid/v2"
)

// Process exit codes per spec §Exit semantics.
const (
	exitOK                  = 0
	exitGenericError        = 1
	exitPreconditionMissing = 2
)

// defaultSeedPath is where the binary appends new INSERT statements.
// Overridable via -seed for tests; see agent-context/lib/watchlist.md §Seed-file writeback.
const defaultSeedPath = "internal/db/seeds/companies.sql"

// perRecordTimeout bounds the full per-record probe budget (dedup query +
// careers GET + ATS probe). Generous enough for Workday pagination on a
// slow tenant; tight enough that one stuck record cannot stall a 200-row run.
// 200 records × 60s = ~3h worst-case stall budget for a single run.
const perRecordTimeout = 60 * time.Second

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable body of main. Returns the process exit code so main's
// only job is os.Exit — keeping the boundary between orchestration and
// process control narrow (matches cmd/batch-enrich convention).
func run(args []string, stdout, stderr io.Writer) int {
	_ = godotenv.Load(".env.local") // no-op if absent; prod sets env vars directly

	logger := slog.New(slog.NewTextHandler(stderr, nil))
	slog.SetDefault(logger)

	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputFlag := fs.String("input", "", "Path to sidecar JSONL file (alternatively pass as positional argument)")
	seedPath := fs.String("seed", defaultSeedPath, "Path to the companies seed SQL file to append to")
	seedGroup := fs.String("group", "", "Optional grouping label for the appended INSERT section comment (e.g. \"GeekWire 200, May 2026\"). Defaults to the run date.")
	noDB := fs.Bool("no-db", false, "Skip the DB dedup check. Use for offline/dry runs.")

	if err := fs.Parse(args); err != nil {
		// flag.ContinueOnError already printed usage on parse failure.
		return exitGenericError
	}

	sidecarPath := *inputFlag
	if sidecarPath == "" {
		if fs.NArg() < 1 {
			fmt.Fprintln(stderr, "[onboard] sidecar path is required (positional arg or -input)")
			return exitGenericError
		}
		sidecarPath = fs.Arg(0)
	}

	group := *seedGroup
	if group == "" {
		group = time.Now().UTC().Format("Jan 2006")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	records, err := readSidecar(sidecarPath)
	if err != nil {
		slog.Error("[onboard] reading sidecar", "error", err)
		return exitGenericError
	}

	// Build the dedup querier. -no-db swaps in a stub that always returns
	// "no match" — useful for an offline dry-run over a sidecar without
	// touching the DB. The stub deliberately does not log per record; the
	// startup log line below is the operator signal.
	var querier dedupQuerier
	var pool *sql.DB
	if *noDB {
		slog.Warn("[onboard] -no-db set: skipping dedup check; duplicate/stale statuses cannot be detected")
		querier = noDBQuerier{}
	} else {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			slog.Error("[onboard] DATABASE_URL is not set (use -no-db to skip dedup)")
			return exitGenericError
		}
		var openErr error
		pool, openErr = sql.Open("pgx", dsn)
		if openErr != nil {
			slog.Error("[onboard] opening database", "error", openErr)
			return exitGenericError
		}
		defer pool.Close()
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := pool.PingContext(pingCtx); err != nil {
			slog.Error("[onboard] pinging database", "error", err)
			return exitGenericError
		}
		querier = db.New(pool)
	}

	// One HTTP client serves both careers-URL probes and ATS adapter calls.
	// No client-level Timeout: per-record context.WithTimeout below is the
	// uniform cancellation source. ATS adapters don't honor client.Timeout
	// uniformly (per-call context is their contract) and we want consistent
	// cancellation semantics across both probe paths.
	httpClient := &http.Client{}

	p := &processor{
		queries:    querier,
		httpClient: httpClient,
		runID:      newRunID(),
		now:        time.Now,
		seedGroup:  group,
	}

	slog.Info("[onboard] starting run",
		"input", sidecarPath,
		"seed_file", *seedPath,
		"run_id", p.runID,
		"records", len(records),
		"no_db", *noDB,
	)

	seedAppender, err := newSeedAppender(*seedPath)
	if err != nil {
		slog.Error("[onboard] loading seed file", "error", err)
		return exitGenericError
	}

	statusCounts := make(map[string]int)
	verifiedCount := 0
	preconditionMisses := []int{}
	var pendingSeed []seedRow

	for i := range records {
		select {
		case <-ctx.Done():
			// Persist what we have so far before exiting. Half-processed
			// records keep their prior state — processRecord only mutates
			// when it reaches a terminal or verified outcome.
			slog.Warn("[onboard] shutdown signal received, halting run", "processed", i, "remaining", len(records)-i)
			goto doneProcessing
		default:
		}

		rec := &records[i]
		recordCtx, cancel := context.WithTimeout(ctx, perRecordTimeout)
		res, err := p.processRecord(recordCtx, rec)
		cancel()
		if err != nil {
			// Transport / DB failures not attributable to a specific
			// company-level annotator gap — spec §Exit semantics maps these
			// to exit 1. Persist whatever progress we've made so the next
			// run resumes cleanly.
			slog.Error("[onboard] processing record", "rank", rec.Rank, "name", rec.Name, "error", err)
			if werr := writeSidecar(sidecarPath, records); werr != nil {
				slog.Error("[onboard] writing sidecar back", "error", werr)
			}
			return exitGenericError
		}

		if res.preconditionMissing != "" {
			preconditionMisses = append(preconditionMisses, rec.Rank)
			slog.Warn("[onboard] precondition missing",
				"rank", rec.Rank,
				"name", rec.Name,
				"missing", res.preconditionMissing,
			)
			continue
		}
		if res.verified {
			verifiedCount++
			if res.seedRow != nil && !seedAppender.Has(res.seedRow.ATS, res.seedRow.BoardToken) {
				pendingSeed = append(pendingSeed, *res.seedRow)
			}
			continue
		}
		if res.status != "" {
			statusCounts[res.status]++
		}
	}
doneProcessing:

	if err := writeSidecar(sidecarPath, records); err != nil {
		slog.Error("[onboard] writing sidecar back", "error", err)
		return exitGenericError
	}

	if err := seedAppender.Append(pendingSeed); err != nil {
		slog.Error("[onboard] appending seed rows", "error", err)
		return exitGenericError
	}

	// Count records still in progress after the run. Distinct from
	// preconditionMisses: a SIGTERM-triggered early break leaves records
	// untouched, and a record with a precondition gap is also in-progress.
	// Both must be reflected in the summary so an operator can tell whether
	// the run actually finished its workload.
	inProgress := 0
	for i := range records {
		if records[i].Status == nil && records[i].VerifiedAt == nil {
			inProgress++
		}
	}

	summary := buildSummary(verifiedCount, statusCounts, preconditionMisses, inProgress, p.runID)
	if err := emitSummary(stdout, summary); err != nil {
		slog.Error("[onboard] emitting summary", "error", err)
		return exitGenericError
	}

	if len(preconditionMisses) > 0 {
		slog.Warn("[onboard] precondition failures; rerun after annotator fills the listed ranks",
			"ranks", preconditionMisses,
		)
		return exitPreconditionMissing
	}
	return exitOK
}

// noDBQuerier satisfies dedupQuerier without touching Postgres. Every
// (ats, board_token) returns "no row found" so the processor treats the
// record as a fresh candidate. Used only when -no-db is passed.
type noDBQuerier struct{}

func (noDBQuerier) FindCompanyDedupStatus(_ context.Context, _ db.FindCompanyDedupStatusParams) (db.FindCompanyDedupStatusRow, error) {
	return db.FindCompanyDedupStatusRow{}, sql.ErrNoRows
}

// summary is the machine-readable run report written to stdout on
// completion. Shape is jq-queryable per the spec's acceptance criteria.
type summary struct {
	RunID                string         `json:"run_id"`
	Verified             int            `json:"verified"`
	ByStatus             map[string]int `json:"by_status"`
	InProgressRemaining  int            `json:"in_progress_remaining"`
	PreconditionFailures []int          `json:"precondition_failures"`
}

func buildSummary(verified int, statusCounts map[string]int, missing []int, inProgress int, runID string) summary {
	// Sort precondition ranks so the output is deterministic for tests and
	// for jq filters that diff successive runs.
	sorted := append([]int(nil), missing...)
	sort.Ints(sorted)
	if statusCounts == nil {
		statusCounts = map[string]int{}
	}
	return summary{
		RunID:                runID,
		Verified:             verified,
		ByStatus:             statusCounts,
		InProgressRemaining:  inProgress,
		PreconditionFailures: sorted,
	}
}

func emitSummary(w io.Writer, s summary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&s); err != nil {
		return fmt.Errorf("encoding summary: %w", err)
	}
	return nil
}

// newRunID returns a ULID for the run, suitable for the verified_run_id
// sidecar field. crypto/rand is the entropy source; failures fall back to
// a timestamp-only id so the run still completes — the id is for provenance,
// not security.
func newRunID() string {
	now := ulid.Timestamp(time.Now().UTC())
	id, err := ulid.New(now, rand.Reader)
	if err != nil {
		// Extremely unlikely (rand.Reader on Linux/Darwin is /dev/urandom or
		// getrandom). Surface a degraded but unique fallback rather than
		// failing the whole run.
		return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}
	return id.String()
}

