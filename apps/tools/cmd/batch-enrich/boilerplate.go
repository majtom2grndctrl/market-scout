// Boilerplate stripping for batch-enrich: groups selected postings by company,
// shells out to ./bin/strip-boilerplate for each company with >=3 postings,
// and substitutes the cleaned description text back into the SelectedPosting
// slice. The 3-posting threshold mirrors the minSamples requirement in
// apps/tools/internal/enrich/boilerplate — Strip needs a corpus to detect prevalence.
// Durable architecture context: agent-context/lib/
//
// Note on the duplicated threshold: apps/tools/cmd/batch-enrich invokes the
// stripper as a subprocess (./bin/strip-boilerplate) and has no compile-time
// dependency on apps/tools/internal/enrich/boilerplate. The constant cannot be
// imported; it must be duplicated and kept in sync manually with minSamples in
// apps/tools/internal/enrich/boilerplate/strip.go.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// stripMinPostings mirrors minSamples in apps/tools/internal/enrich/boilerplate/strip.go;
// no import path exists — keep in sync manually.
const stripMinPostings = 3

// stripBinaryPath is the relative path to the prebuilt strip-boilerplate
// helper. batch-enrich runs from apps/tools/, so this resolves to
// apps/tools/bin/strip-boilerplate.
const stripBinaryPath = "./bin/strip-boilerplate"

type stripInput struct {
	CompanyID   int64   `json:"company_id"`
	SelectedIDs []int64 `json:"selected_ids"`
}

type stripOutput struct {
	Postings []stripPosting `json:"postings"`
}

// stripPosting is one row of the strip-boilerplate subprocess output, also
// the unit the stripRunner seam returns. Fields mirror the subprocess JSON
// shape (`posting_id`, `cleaned_text`) so the real runner can decode straight
// into the seam type without a translation step.
type stripPosting struct {
	PostingID   int64  `json:"posting_id"`
	CleanedText string `json:"cleaned_text"`
}

// stripRunner abstracts the strip-boilerplate subprocess so tests can
// substitute a fake. Mirrors the agentRunner seam in dispatch.go.
type stripRunner interface {
	Run(ctx context.Context, companyID int64, selectedIDs []int64) (postings []stripPosting, err error)
}

// execStripRunner is the production stripRunner: it shells out to the
// prebuilt strip-boilerplate binary and decodes its JSON output.
type execStripRunner struct {
	binaryPath string
	timeout    time.Duration
}

// newExecStripRunner returns the production stripRunner. A zero timeout
// disables the per-call cap — Run will pass the caller's context through
// unwrapped, matching the claudeRunner convention in dispatch.go.
func newExecStripRunner(binaryPath string, timeout time.Duration) stripRunner {
	return &execStripRunner{binaryPath: binaryPath, timeout: timeout}
}

// Run invokes the strip-boilerplate binary for a single company and returns
// the decoded posting list. Non-zero exit or context cancellation surfaces
// as an error.
func (r *execStripRunner) Run(ctx context.Context, companyID int64, selectedIDs []int64) ([]stripPosting, error) {
	payload, err := json.Marshal(stripInput{
		CompanyID:   companyID,
		SelectedIDs: selectedIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("strip-boilerplate: encoding input for company %d: %w", companyID, err)
	}

	// Per-call timeout when configured; otherwise the caller's ctx flows
	// through unwrapped so we don't allocate a derived context per company.
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, r.binaryPath)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, fmt.Errorf("strip-boilerplate: %w", ctxErr)
			}
			return nil, fmt.Errorf("strip-boilerplate: exit status %d: %s", exitErr.ExitCode(), stderr.String())
		}
		return nil, fmt.Errorf("strip-boilerplate: %w: %s", err, stderr.String())
	}

	var out stripOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("strip-boilerplate: parsing output for company %d: %w", companyID, err)
	}
	return out.Postings, nil
}

// StripBoilerplate groups postings by company_id, calls the supplied runner
// for each company with >=3 postings, and returns a new slice of
// SelectedPostings with cleaned_text substituted into DescriptionText where
// available.
//
// For companies below the threshold (< 3 postings), postings pass through unchanged.
// Runner errors abort the run.
// If the runner returns no entry or an empty cleaned_text for a posting,
// falls back to the original DescriptionText and logs a warning.
func StripBoilerplate(ctx context.Context, runner stripRunner, postings []SelectedPosting) ([]SelectedPosting, error) {
	// Group posting indices by company so we preserve the original ordering
	// when we rebuild the output slice.
	indicesByCompany := make(map[int64][]int, len(postings))
	companyOrder := make([]int64, 0, len(postings))
	for i, p := range postings {
		if _, seen := indicesByCompany[p.CompanyID]; !seen {
			companyOrder = append(companyOrder, p.CompanyID)
		}
		indicesByCompany[p.CompanyID] = append(indicesByCompany[p.CompanyID], i)
	}

	result := make([]SelectedPosting, len(postings))
	copy(result, postings)

	for _, companyID := range companyOrder {
		indices := indicesByCompany[companyID]
		if len(indices) < stripMinPostings {
			continue
		}

		selectedIDs := make([]int64, len(indices))
		for j, idx := range indices {
			selectedIDs[j] = postings[idx].PostingID
		}

		runnerPostings, err := runner.Run(ctx, companyID, selectedIDs)
		if err != nil {
			return nil, err
		}

		cleanedByID := make(map[int64]string, len(runnerPostings))
		for _, p := range runnerPostings {
			cleanedByID[p.PostingID] = p.CleanedText
		}

		for _, idx := range indices {
			postingID := postings[idx].PostingID
			cleaned, ok := cleanedByID[postingID]
			if !ok {
				slog.Warn("[batch-enrich] strip-boilerplate returned no entry for posting — using original description",
					"posting_id", postingID,
				)
				continue
			}
			if cleaned == "" {
				slog.Warn("[batch-enrich] strip-boilerplate returned empty cleaned_text — using original description",
					"posting_id", postingID,
				)
				continue
			}
			result[idx].DescriptionText = cleaned
		}

		slog.Info("[batch-enrich] stripped boilerplate",
			"company_id", companyID,
			"posting_count", len(indices),
		)
	}

	return result, nil
}
