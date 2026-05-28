// Run report and failures.jsonl emission for batch-enrich. Builds a
// RunReport from the in-memory results slice and renders it as JSON or
// Markdown to an io.Writer; appends one JSON line per JSON- or
// validation-failed posting to agent-output/batch-enrich/failures.jsonl.
// DB failures are excluded from failures.jsonl (infrastructure events, not
// retryable agent output) but do appear in RunReport.Failures written to
// stdout.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// failuresFilePath is the relative path where AppendFailures writes its
// JSON-lines log. batch-enrich runs from apps/tools/, so this resolves to
// apps/tools/agent-output/batch-enrich/failures.jsonl.
// Declared as a var so tests can redirect writes to a temp directory.
var failuresFilePath = "agent-output/batch-enrich/failures.jsonl"

// RunReport is the top-level structure emitted at end of run. The shape
// mirrors the operator-facing fields documented in the batch-enrich spec.
type RunReport struct {
	RunParams        RunParams        `json:"run_params"`
	Counts           RunCounts        `json:"counts"`
	NewTaxonomy      NewTaxonomy      `json:"new_taxonomy"`
	PostingSummaries []PostingSummary `json:"posting_summaries"`
	Failures         []FailureSummary `json:"failures"`
}

// RunParams captures the resolved runtime knobs and pinned identifiers for
// the run so a report alone is enough to reproduce the invocation.
type RunParams struct {
	PromptVersion string `json:"prompt_version"`
	Model         string `json:"model"`
	WaveSize      int    `json:"wave_size"`
	MaxRetries    int    `json:"max_retries"`
	Count         int    `json:"count"`
	Focus         string `json:"focus,omitempty"`
	Force         bool   `json:"force"`
	ReportFormat  string `json:"report_format"`
}

// RunCounts is the per-outcome tally for one run. SkippedNoDescription is
// always zero in this implementation — the selection query filters with
// WHERE description_text IS NOT NULL, so NULL-description postings never
// reach the dispatcher. The field is kept to make the zero explicit for JSON
// consumers rather than absent.
type RunCounts struct {
	Selected             int `json:"selected"`
	Dispatched           int `json:"dispatched"`
	Enriched             int `json:"enriched"`
	JSONFailed           int `json:"json_failed"`
	ValidationFailed     int `json:"validation_failed"`
	DBFailed             int `json:"db_failed"`
	SkippedNoDescription int `json:"skipped_no_description"`
	ReEnriched           int `json:"re_enriched"`
}

// NewTaxonomy lists slugs that the run materialised — present in taxAfter
// but absent in taxBefore. The three buckets correspond to the three taxonomy
// tables writeback inserts into; role_dimensions is never minted by the agent.
type NewTaxonomy struct {
	CanonicalRoles  []NewTaxonomyEntry `json:"canonical_roles"`
	Specializations []NewTaxonomyEntry `json:"specializations"`
	Skills          []NewTaxonomyEntry `json:"skills"`
}

// NewTaxonomyEntry records a freshly-minted taxonomy row alongside the
// posting that first emitted the slug, so operators can audit provenance.
type NewTaxonomyEntry struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Source int64  `json:"source_posting_id"`
}

// PostingSummary is one row in the per-posting section of the report. The
// Summary field is the agent's free-text summary; LastReason carries the
// final validation or JSON failure message for non-enriched outcomes.
type PostingSummary struct {
	PostingID  int64   `json:"posting_id"`
	Title      string  `json:"title"`
	Outcome    Outcome `json:"outcome"`
	Summary    string  `json:"summary,omitempty"`
	LastReason string  `json:"last_failure_reason,omitempty"`
}

// FailureSummary is one row in the failures section of the report (in-memory
// only; the failures.jsonl file uses a different shape — see FailureLine).
type FailureSummary struct {
	PostingID       int64   `json:"posting_id"`
	Outcome         Outcome `json:"outcome"`
	LastReason      string  `json:"reason"`
	LastRawResponse string  `json:"last_raw_response"`
}

// FailureLine is one record in failures.jsonl. Includes per-run provenance
// (timestamp, prompt_version, model) so an operator inspecting the file can
// correlate without consulting the report.
type FailureLine struct {
	RunTimestamp  string  `json:"run_timestamp"`
	PostingID     int64   `json:"posting_id"`
	Attempt       int     `json:"attempt"`
	Outcome       Outcome `json:"outcome"`
	Reason        string  `json:"reason"`
	RawResponse   string  `json:"raw_response"`
	PromptVersion string  `json:"prompt_version"`
	Model         string  `json:"model"`
	AttemptedAt   string  `json:"attempted_at"`
}

// BuildReport assembles a RunReport from run results plus before/after
// taxonomy snapshots. selectedCount is the number of postings returned by
// the selection query, before any dispatch or filtering — distinct from
// len(results), which is the count actually dispatched to agents.
// alreadyClassified is the subset of selected posting IDs that already had
// a classifications row before this run — used to fill the ReEnriched count
// under --force.
func BuildReport(cfg Config, selectedCount int, results []PostingResult, taxBefore Taxonomy, taxAfter Taxonomy, alreadyClassified []int64) RunReport {
	rep := RunReport{
		RunParams: RunParams{
			PromptVersion: cfg.PromptVersion,
			Model:         cfg.Model,
			WaveSize:      cfg.WaveSize,
			MaxRetries:    cfg.MaxRetries,
			Count:         cfg.Count,
			Focus:         cfg.Focus,
			Force:         cfg.Force,
			ReportFormat:  cfg.ReportFormat,
		},
	}

	rep.Counts.Selected = selectedCount
	rep.Counts.Dispatched = len(results)

	preExisting := make(map[int64]struct{}, len(alreadyClassified))
	for _, id := range alreadyClassified {
		preExisting[id] = struct{}{}
	}

	rep.PostingSummaries = make([]PostingSummary, 0, len(results))
	for _, r := range results {
		// Cancelled postings never reached the agent (the goroutine bailed
		// out at the semaphore before dispatch). They are stamped as
		// json_failed for AppendFailures' filter, but they are not retryable
		// agent output and must not inflate the json_failed count. They
		// still appear in posting_summaries so the operator can see what
		// the run touched.
		cancelled := r.LastReason == reasonCancelled

		switch r.Outcome {
		case OutcomeEnriched:
			rep.Counts.Enriched++
			if _, was := preExisting[r.PostingID]; was {
				rep.Counts.ReEnriched++
			}
		case OutcomeJSONFailed:
			if !cancelled {
				rep.Counts.JSONFailed++
			}
		case OutcomeValidationFailed:
			if !cancelled {
				rep.Counts.ValidationFailed++
			}
		case OutcomeDBFailed:
			if !cancelled {
				rep.Counts.DBFailed++
			}
		}

		rep.PostingSummaries = append(rep.PostingSummaries, PostingSummary{
			PostingID:  r.PostingID,
			Title:      r.Title,
			Outcome:    r.Outcome,
			Summary:    r.Summary,
			LastReason: r.LastReason,
		})

		if (r.Outcome == OutcomeJSONFailed || r.Outcome == OutcomeValidationFailed || r.Outcome == OutcomeDBFailed) && !cancelled {
			rep.Failures = append(rep.Failures, FailureSummary{
				PostingID:       r.PostingID,
				Outcome:         r.Outcome,
				LastReason:      r.LastReason,
				LastRawResponse: r.LastRawResponse,
			})
		}
	}

	rep.NewTaxonomy = diffTaxonomy(results, taxBefore, taxAfter)
	return rep
}

// diffTaxonomy computes slugs present in taxAfter but missing in taxBefore
// for each of canonical_roles, specializations, and skills. The
// source_posting_id is the first enriched posting whose AgentResponse
// includes the slug; if no result claims the slug (shouldn't happen) the
// source is recorded as 0.
func diffTaxonomy(results []PostingResult, before, after Taxonomy) NewTaxonomy {
	return NewTaxonomy{
		CanonicalRoles:  diffEntries(before.CanonicalRoles, after.CanonicalRoles, results, taxKindCanonicalRole),
		Specializations: diffEntries(before.Specializations, after.Specializations, results, taxKindSpecialization),
		Skills:          diffEntries(before.Skills, after.Skills, results, taxKindSkill),
	}
}

// taxKind identifies which agent-response slot to scan when attributing a
// freshly-minted slug back to the posting that introduced it.
type taxKind int

const (
	taxKindCanonicalRole taxKind = iota
	taxKindSpecialization
	taxKindSkill
)

func diffEntries(before, after map[string]TaxonomyEntry, results []PostingResult, kind taxKind) []NewTaxonomyEntry {
	if len(after) == 0 {
		return nil
	}
	newSlugs := make([]string, 0)
	for slug := range after {
		if _, existed := before[slug]; !existed {
			newSlugs = append(newSlugs, slug)
		}
	}
	if len(newSlugs) == 0 {
		return nil
	}
	sort.Strings(newSlugs)

	out := make([]NewTaxonomyEntry, 0, len(newSlugs))
	for _, slug := range newSlugs {
		entry := after[slug]
		out = append(out, NewTaxonomyEntry{
			Slug:   slug,
			Name:   entry.Name,
			Source: findSourcePosting(results, slug, kind),
		})
	}
	return out
}

// findSourcePosting walks results in order and returns the first enriched
// posting whose AgentResponse names slug in the matching slot. Returns 0 if
// no result claims it — defensive only; in practice writeback would not have
// inserted a slug nobody emitted.
func findSourcePosting(results []PostingResult, slug string, kind taxKind) int64 {
	for _, r := range results {
		if r.Classification == nil {
			continue
		}
		resp := r.Classification.AgentResponse
		switch kind {
		case taxKindCanonicalRole:
			for _, role := range resp.CanonicalRoles {
				if role.Slug == slug {
					return r.PostingID
				}
			}
		case taxKindSpecialization:
			for _, spec := range resp.Specializations {
				if spec.Slug == slug {
					return r.PostingID
				}
			}
		case taxKindSkill:
			for _, sk := range resp.Skills {
				if sk.Slug == slug {
					return r.PostingID
				}
			}
		}
	}
	return 0
}

// EmitReport renders the report to w in the requested format. "json" emits
// a single pretty-printed JSON object; "markdown" emits a human-scannable
// rendering with heading, param table, counts, and per-posting list. An
// unknown format value returns an error.
func EmitReport(w io.Writer, report RunReport, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "markdown":
		return emitMarkdown(w, report)
	default:
		return fmt.Errorf("unknown report format %q", format)
	}
}

func emitMarkdown(w io.Writer, r RunReport) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# batch-enrich run report\n\n")

	fmt.Fprintf(&sb, "## Run params\n\n")
	fmt.Fprintf(&sb, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| prompt_version | %s |\n", r.RunParams.PromptVersion)
	fmt.Fprintf(&sb, "| model | %s |\n", r.RunParams.Model)
	fmt.Fprintf(&sb, "| wave_size | %d |\n", r.RunParams.WaveSize)
	fmt.Fprintf(&sb, "| max_retries | %d |\n", r.RunParams.MaxRetries)
	fmt.Fprintf(&sb, "| count | %d |\n", r.RunParams.Count)
	fmt.Fprintf(&sb, "| focus | %s |\n", r.RunParams.Focus)
	fmt.Fprintf(&sb, "| force | %t |\n", r.RunParams.Force)
	fmt.Fprintf(&sb, "| report_format | %s |\n\n", r.RunParams.ReportFormat)

	fmt.Fprintf(&sb, "## Counts\n\n")
	fmt.Fprintf(&sb, "| Outcome | Count |\n|---|---|\n")
	fmt.Fprintf(&sb, "| selected | %d |\n", r.Counts.Selected)
	fmt.Fprintf(&sb, "| dispatched | %d |\n", r.Counts.Dispatched)
	fmt.Fprintf(&sb, "| enriched | %d |\n", r.Counts.Enriched)
	fmt.Fprintf(&sb, "| json_failed | %d |\n", r.Counts.JSONFailed)
	fmt.Fprintf(&sb, "| validation_failed | %d |\n", r.Counts.ValidationFailed)
	fmt.Fprintf(&sb, "| db_failed | %d |\n", r.Counts.DBFailed)
	fmt.Fprintf(&sb, "| skipped_no_description | %d |\n", r.Counts.SkippedNoDescription)
	fmt.Fprintf(&sb, "| re_enriched | %d |\n\n", r.Counts.ReEnriched)

	if len(r.NewTaxonomy.CanonicalRoles) > 0 || len(r.NewTaxonomy.Specializations) > 0 || len(r.NewTaxonomy.Skills) > 0 {
		fmt.Fprintf(&sb, "## New taxonomy\n\n")
		writeTaxList(&sb, "Canonical roles", r.NewTaxonomy.CanonicalRoles)
		writeTaxList(&sb, "Specializations", r.NewTaxonomy.Specializations)
		writeTaxList(&sb, "Skills", r.NewTaxonomy.Skills)
	}

	fmt.Fprintf(&sb, "## Postings\n\n")
	for _, p := range r.PostingSummaries {
		fmt.Fprintf(&sb, "- **%d** [%s] %s\n", p.PostingID, p.Outcome, p.Title)
		if p.Summary != "" {
			fmt.Fprintf(&sb, "    summary: %s\n", p.Summary)
		}
		if p.LastReason != "" {
			fmt.Fprintf(&sb, "    reason: %s\n", p.LastReason)
		}
	}

	_, err := w.Write([]byte(sb.String()))
	return err
}

func writeTaxList(sb *strings.Builder, heading string, entries []NewTaxonomyEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(sb, "### %s\n\n", heading)
	for _, e := range entries {
		fmt.Fprintf(sb, "- %s (%s) — first seen on posting %d\n", e.Slug, e.Name, e.Source)
	}
	sb.WriteByte('\n')
}

// AppendFailures appends one JSON line per json_failed or validation_failed
// posting to failures.jsonl. The parent directory and file are created on
// demand; prior lines are never truncated. DB failures are intentionally
// skipped — they are infrastructure events, not agent misbehaviour worth
// replaying through the retry path.
func AppendFailures(results []PostingResult, runTimestamp string, cfg Config) error {
	// Filter first so we can skip the file open entirely on a clean run.
	var lines []FailureLine
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range results {
		if r.Outcome != OutcomeJSONFailed && r.Outcome != OutcomeValidationFailed {
			continue
		}
		// Cancelled postings never reached the agent, so their failure is not
		// retryable agent output. Skip them to avoid polluting the retry corpus.
		if r.LastReason == reasonCancelled {
			continue
		}
		lines = append(lines, FailureLine{
			RunTimestamp:  runTimestamp,
			PostingID:     r.PostingID,
			Attempt:       r.Attempts,
			Outcome:       r.Outcome,
			Reason:        r.LastReason,
			RawResponse:   r.LastRawResponse,
			PromptVersion: cfg.PromptVersion,
			Model:         cfg.Model,
			AttemptedAt:   now,
		})
	}
	if len(lines) == 0 {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(failuresFilePath), 0o755); err != nil {
		return fmt.Errorf("creating failures dir: %w", err)
	}
	f, err := os.OpenFile(failuresFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening failures file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Error("[batch-enrich] closing failures.jsonl", "error", err)
		}
	}()

	enc := json.NewEncoder(f)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			return fmt.Errorf("encoding failure line for posting %d: %w", line.PostingID, err)
		}
	}
	return nil
}
