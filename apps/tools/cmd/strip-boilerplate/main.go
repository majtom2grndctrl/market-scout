// Command strip-boilerplate reads a company id and a list of selected posting
// ids on stdin, fetches the company's full description corpus, runs the
// boilerplate stripper across it, and writes cleaned text for the selected
// postings to stdout. Invoked by cmd/batch-enrich —
// one process per company with >=3 selected postings — boilerplate.Strip requires at least
// minSamples inputs to make a prevalence determination.
// See: agent-context/lib/project.md
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/enrich/boilerplate"
)

type input struct {
	CompanyID   int64   `json:"company_id"`
	SelectedIDs []int64 `json:"selected_ids"`
}

type outputPosting struct {
	PostingID   int64  `json:"posting_id"`
	CleanedText string `json:"cleaned_text"`
}

type output struct {
	Postings []outputPosting `json:"postings"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("[strip-boilerplate] fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load(".env.local") // no-op if absent; prod sets env vars directly

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rawInput, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	var in input
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return fmt.Errorf("parsing stdin JSON: %w", err)
	}
	if in.CompanyID == 0 {
		return errors.New("company_id is required and must be non-zero")
	}

	pool, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer pool.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := pool.PingContext(pingCtx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}

	queries := db.New(pool)
	rows, err := queries.ListLatestDescriptionsByCompany(ctx, in.CompanyID)
	if err != nil {
		return fmt.Errorf("loading descriptions for company %d: %w", in.CompanyID, err)
	}

	slog.Info("[strip-boilerplate] loaded corpus",
		"company_id", in.CompanyID,
		"corpus_size", len(rows),
		"selected", len(in.SelectedIDs),
	)

	// Build the corpus in a deterministic order, then strip. Strip preserves
	// input ordering, so the index of each posting maps cleanly back to its id.
	descriptions := make([]string, len(rows))
	idByIndex := make([]int64, len(rows))
	for i, r := range rows {
		descriptions[i] = r.DescriptionText
		idByIndex[i] = r.JobPostingID
	}

	cleaned := boilerplate.Strip(descriptions)
	if len(cleaned) != len(descriptions) {
		return fmt.Errorf("boilerplate.Strip returned %d entries for %d inputs", len(cleaned), len(descriptions))
	}

	cleanedByID := make(map[int64]string, len(cleaned))
	for i, id := range idByIndex {
		cleanedByID[id] = cleaned[i]
	}

	out := output{Postings: make([]outputPosting, 0, len(in.SelectedIDs))}
	missing := 0
	for _, id := range in.SelectedIDs {
		text, ok := cleanedByID[id]
		if !ok {
			missing++
		}
		out.Postings = append(out.Postings, outputPosting{
			PostingID:   id,
			CleanedText: text, // empty string when not found in corpus
		})
	}

	if missing > 0 {
		slog.Warn("[strip-boilerplate] selected postings not in corpus",
			"company_id", in.CompanyID,
			"missing", missing,
		)
	}

	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encoding output: %w", err)
	}

	slog.Info("[strip-boilerplate] done",
		"company_id", in.CompanyID,
		"emitted", len(out.Postings),
	)
	return nil
}
