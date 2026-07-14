package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunReport_RoundTripsAndShape verifies BuildReport produces a structure
// that json.Marshal/Unmarshal round-trips losslessly and that the wire shape
// uses the operator-facing key names from the spec.
func TestRunReport_RoundTripsAndShape(t *testing.T) {
	cfg := Config{
		PromptVersion: PromptVersion,
		Runner:        RunnerCodexExec,
		Model:         CodexExecModel,
		WaveSize:      10,
		MaxRetries:    3,
		Count:         3,
		ReportFormat:  "json",
	}

	enriched := PostingResult{
		PostingID: 1,
		CompanyID: 100,
		Title:     "Software Engineer",
		Outcome:   OutcomeEnriched,
		Summary:   "Senior software engineer role.",
		Attempts:  1,
		Classification: &ValidatedClassification{
			AgentResponse: AgentResponse{
				PostingID: 1,
				Classification: AgentClassification{
					Seniority: "senior",
				},
				CanonicalRoles: []AgentCanonicalRole{
					{Slug: "software-engineer", Name: "Software Engineer", Dimensions: []string{"ic", "engineering"}},
				},
				Summary: "Senior software engineer role.",
			},
		},
	}
	jsonFailed := PostingResult{
		PostingID:       2,
		CompanyID:       100,
		Title:           "Designer",
		Outcome:         OutcomeJSONFailed,
		LastReason:      "json parse: unexpected token",
		LastRawResponse: "{not json",
		Attempts:        4,
	}
	validationFailed := PostingResult{
		PostingID:       3,
		CompanyID:       100,
		Title:           "PM",
		Outcome:         OutcomeValidationFailed,
		LastReason:      "`Bad_Slug` is not a valid slug",
		LastRawResponse: `{"posting_id":3}`,
		Attempts:        4,
	}

	results := []PostingResult{enriched, jsonFailed, validationFailed}

	taxBefore := newPromptTaxonomy()
	taxAfter := newPromptTaxonomy()

	report := BuildReport(cfg, len(results), results, taxBefore, taxAfter, nil)

	// Counts match the fixtures we passed in.
	if report.Counts.Enriched != 1 {
		t.Errorf("enriched: want 1, got %d", report.Counts.Enriched)
	}
	if report.Counts.JSONFailed != 1 {
		t.Errorf("json_failed: want 1, got %d", report.Counts.JSONFailed)
	}
	if report.Counts.ValidationFailed != 1 {
		t.Errorf("validation_failed: want 1, got %d", report.Counts.ValidationFailed)
	}
	if report.Counts.DBFailed != 0 {
		t.Errorf("db_failed: want 0, got %d", report.Counts.DBFailed)
	}

	// Failures slice should hold the two non-enriched, non-DB results.
	if len(report.Failures) != 2 {
		t.Fatalf("failures: want 2, got %d (%+v)", len(report.Failures), report.Failures)
	}

	// Marshal and check the wire-level key names.
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rawStr := string(raw)
	wantKeys := []string{
		`"run_params"`,
		`"runner"`,
		`"counts"`,
		`"posting_summaries"`,
		`"failures"`,
	}
	for _, k := range wantKeys {
		if !strings.Contains(rawStr, k) {
			t.Errorf("expected key %s in JSON output, got %s", k, rawStr)
		}
	}

	// Round-trip back into a second RunReport — counts should match.
	var roundTripped RunReport
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundTripped.Counts != report.Counts {
		t.Errorf("counts changed across round-trip:\nbefore: %+v\nafter:  %+v", report.Counts, roundTripped.Counts)
	}
	if len(roundTripped.Failures) != len(report.Failures) {
		t.Errorf("failures len mismatch after round-trip: want %d got %d", len(report.Failures), len(roundTripped.Failures))
	}
	if len(roundTripped.PostingSummaries) != len(report.PostingSummaries) {
		t.Errorf("posting_summaries len mismatch after round-trip")
	}
	if roundTripped.RunParams.PromptVersion != cfg.PromptVersion {
		t.Errorf("prompt_version lost across round-trip: %q", roundTripped.RunParams.PromptVersion)
	}
	if roundTripped.RunParams.Runner != cfg.Runner {
		t.Errorf("runner lost across round-trip: got %q, want %q", roundTripped.RunParams.Runner, cfg.Runner)
	}
}

// setFailuresFilePath redirects AppendFailures output to path for the
// duration of the test and restores the original value via t.Cleanup.
func setFailuresFilePath(t *testing.T, path string) {
	t.Helper()
	orig := failuresFilePath
	failuresFilePath = path
	t.Cleanup(func() { failuresFilePath = orig })
}

// readNonEmptyLines reads all non-empty lines from a file. Returns nil if
// the file does not exist.
func readNonEmptyLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanning %s: %v", path, err)
	}
	return lines
}

func minimalCfg() Config {
	return Config{PromptVersion: PromptVersion, Runner: RunnerCodexExec, Model: CodexExecModel}
}

// TestAppendFailures_WritesValidJSONL verifies that json_failed and
// validation_failed results are written as valid JSON lines with correct
// posting_id and outcome values, while db_failed results are excluded.
func TestAppendFailures_WritesValidJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failures.jsonl")
	setFailuresFilePath(t, path)

	results := []PostingResult{
		{PostingID: 10, Outcome: OutcomeJSONFailed, LastReason: "unexpected token", LastRawResponse: "{bad}", Attempts: 2},
		{PostingID: 20, Outcome: OutcomeValidationFailed, LastReason: "bad slug", LastRawResponse: `{"posting_id":20}`, Attempts: 1},
		{PostingID: 30, Outcome: OutcomeDBFailed, LastReason: "connection refused", Attempts: 1},
	}

	if err := AppendFailures(results, "2026-05-13T00:00:00Z", minimalCfg()); err != nil {
		t.Fatalf("AppendFailures: %v", err)
	}

	lines := readNonEmptyLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %v", len(lines), lines)
	}

	wantByID := map[int64]Outcome{10: OutcomeJSONFailed, 20: OutcomeValidationFailed}
	for _, raw := range lines {
		var fl FailureLine
		if err := json.Unmarshal([]byte(raw), &fl); err != nil {
			t.Fatalf("invalid JSON line %q: %v", raw, err)
		}
		want, ok := wantByID[fl.PostingID]
		if !ok {
			t.Errorf("unexpected posting_id %d in failures.jsonl", fl.PostingID)
			continue
		}
		if fl.Outcome != want {
			t.Errorf("posting %d: want outcome %q, got %q", fl.PostingID, want, fl.Outcome)
		}
		delete(wantByID, fl.PostingID)
	}
	for id := range wantByID {
		t.Errorf("expected posting_id %d in failures.jsonl but it was absent", id)
	}
}

// TestAppendFailures_CreatesParentDir verifies that AppendFailures creates
// the parent directory when it does not already exist.
func TestAppendFailures_CreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "failures.jsonl")
	setFailuresFilePath(t, path)

	results := []PostingResult{
		{PostingID: 1, Outcome: OutcomeJSONFailed, LastReason: "bad json", Attempts: 1},
	}

	if err := AppendFailures(results, "2026-05-13T00:00:00Z", minimalCfg()); err != nil {
		t.Fatalf("AppendFailures: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist at %s: %v", path, err)
	}
}

// TestAppendFailures_AppendWithoutTruncation verifies that calling
// AppendFailures twice accumulates lines — prior lines are never truncated.
func TestAppendFailures_AppendWithoutTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failures.jsonl")
	setFailuresFilePath(t, path)

	cfg := minimalCfg()
	first := []PostingResult{
		{PostingID: 100, Outcome: OutcomeJSONFailed, LastReason: "first call", Attempts: 1},
	}
	second := []PostingResult{
		{PostingID: 200, Outcome: OutcomeJSONFailed, LastReason: "second call", Attempts: 1},
	}

	if err := AppendFailures(first, "2026-05-13T00:00:00Z", cfg); err != nil {
		t.Fatalf("first AppendFailures: %v", err)
	}
	if err := AppendFailures(second, "2026-05-13T00:00:01Z", cfg); err != nil {
		t.Fatalf("second AppendFailures: %v", err)
	}

	lines := readNonEmptyLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines after two calls, got %d", len(lines))
	}

	ids := make(map[int64]bool)
	for _, raw := range lines {
		var fl FailureLine
		if err := json.Unmarshal([]byte(raw), &fl); err != nil {
			t.Fatalf("invalid JSON line %q: %v", raw, err)
		}
		ids[fl.PostingID] = true
	}
	if !ids[100] {
		t.Error("posting 100 missing after second append — prior lines were truncated")
	}
	if !ids[200] {
		t.Error("posting 200 missing after second append")
	}
}

// TestAppendFailures_SkipsCancelled verifies that a json_failed result with
// LastReason == "cancelled" is not written to failures.jsonl. Cancelled
// postings never reached the agent and are not retryable output.
func TestAppendFailures_SkipsCancelled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failures.jsonl")
	setFailuresFilePath(t, path)

	results := []PostingResult{
		{PostingID: 42, Outcome: OutcomeJSONFailed, LastReason: "cancelled", Attempts: 0},
	}

	if err := AppendFailures(results, "2026-05-13T00:00:00Z", minimalCfg()); err != nil {
		t.Fatalf("AppendFailures: %v", err)
	}

	// AppendFailures skips all lines, so the file must not be created at all.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		lines := readNonEmptyLines(t, path)
		if len(lines) != 0 {
			t.Errorf("cancelled posting must not appear in failures.jsonl; got %d lines: %v", len(lines), lines)
		}
	}
}

// TestBuildReport_CancelledDBFailedNotCounted verifies that an OutcomeDBFailed
// result stamped with reasonCancelled (written by WriteBack on context
// cancellation) does not increment Counts.DBFailed and does not appear in
// rep.Failures. The posting still appears in PostingSummaries so the operator
// can see what the run touched.
func TestBuildReport_CancelledDBFailedNotCounted(t *testing.T) {
	cfg := minimalCfg()

	cancelledDB := PostingResult{
		PostingID:  99,
		CompanyID:  1,
		Title:      "PM",
		Outcome:    OutcomeDBFailed,
		LastReason: reasonCancelled,
		Attempts:   0,
	}
	realDB := PostingResult{
		PostingID:  100,
		CompanyID:  1,
		Title:      "SWE",
		Outcome:    OutcomeDBFailed,
		LastReason: "connection refused",
		Attempts:   1,
	}

	results := []PostingResult{cancelledDB, realDB}
	tax := newPromptTaxonomy()

	report := BuildReport(cfg, len(results), results, tax, tax, nil)

	// Only the real (non-cancelled) DBFailed should contribute to the count.
	if report.Counts.DBFailed != 1 {
		t.Errorf("db_failed count: want 1, got %d", report.Counts.DBFailed)
	}

	// Failures slice should contain only the real DBFailed entry.
	if len(report.Failures) != 1 {
		t.Fatalf("failures: want 1, got %d (%+v)", len(report.Failures), report.Failures)
	}
	if report.Failures[0].PostingID != 100 {
		t.Errorf("failures[0].PostingID: want 100, got %d", report.Failures[0].PostingID)
	}

	// Both postings should still appear in PostingSummaries.
	if len(report.PostingSummaries) != 2 {
		t.Errorf("posting_summaries: want 2, got %d", len(report.PostingSummaries))
	}
}

// TestDiffTaxonomy_CapturesNewSlugs verifies that diffTaxonomy reports only
// slugs present in `after` but missing in `before`, sourced from the result
// that introduced them. Slugs in both maps must be omitted from the diff.
func TestDiffTaxonomy_CapturesNewSlugs(t *testing.T) {
	before := Taxonomy{
		CanonicalRoles: map[string]TaxonomyEntry{
			"software-engineer": {ID: 1, Name: "Software Engineer"},
		},
		Specializations: map[string]TaxonomyEntry{},
		Skills:          map[string]TaxonomyEntry{},
	}
	after := Taxonomy{
		CanonicalRoles: map[string]TaxonomyEntry{
			"software-engineer": {ID: 1, Name: "Software Engineer"}, // pre-existing
			"design-engineer":   {ID: 2, Name: "Design Engineer"},   // new
		},
		Specializations: map[string]TaxonomyEntry{},
		Skills: map[string]TaxonomyEntry{
			"go": {ID: 30, Name: "Go"}, // new
		},
	}

	results := []PostingResult{
		{
			PostingID: 42,
			Outcome:   OutcomeEnriched,
			Classification: &ValidatedClassification{
				AgentResponse: AgentResponse{
					CanonicalRoles: []AgentCanonicalRole{
						{Slug: "design-engineer", Name: "Design Engineer", Dimensions: []string{"ic"}},
					},
					Skills: []AgentSkill{
						{Slug: "go", Name: "Go", Requirement: "required"},
					},
				},
			},
		},
	}

	diff := diffTaxonomy(results, before, after)

	if len(diff.CanonicalRoles) != 1 {
		t.Fatalf("want 1 new canonical role, got %d (%+v)", len(diff.CanonicalRoles), diff.CanonicalRoles)
	}
	if diff.CanonicalRoles[0].Slug != "design-engineer" {
		t.Errorf("want slug design-engineer, got %q", diff.CanonicalRoles[0].Slug)
	}
	if diff.CanonicalRoles[0].Source != 42 {
		t.Errorf("want source posting 42, got %d", diff.CanonicalRoles[0].Source)
	}
	// software-engineer was in both before and after — must not appear.
	for _, e := range diff.CanonicalRoles {
		if e.Slug == "software-engineer" {
			t.Errorf("pre-existing slug software-engineer should not appear in diff")
		}
	}

	if len(diff.Skills) != 1 || diff.Skills[0].Slug != "go" {
		t.Errorf("want new skill go, got %+v", diff.Skills)
	}
	if len(diff.Specializations) != 0 {
		t.Errorf("want no new specializations, got %+v", diff.Specializations)
	}
}

// TestEmitReport_JSONIsValid verifies the json format renders a well-formed
// document with the operator-facing top-level keys.
func TestEmitReport_JSONIsValid(t *testing.T) {
	report := RunReport{
		RunParams: RunParams{
			Runner:        RunnerCodexExec,
			PromptVersion: PromptVersion,
			Model:         CodexExecModel,
			ReportFormat:  "json",
		},
		Counts:           RunCounts{Selected: 1, Dispatched: 1, Enriched: 1},
		PostingSummaries: []PostingSummary{{PostingID: 1, Title: "SWE", Outcome: OutcomeEnriched}},
	}

	var buf bytes.Buffer
	if err := EmitReport(&buf, report, "json"); err != nil {
		t.Fatalf("EmitReport: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	for _, key := range []string{"run_params", "counts", "posting_summaries"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("missing top-level key %q in JSON output", key)
		}
	}
}

// TestEmitReport_MarkdownIsNonEmpty verifies the markdown format emits
// recognisable output without panicking on a populated report.
func TestEmitReport_MarkdownIsNonEmpty(t *testing.T) {
	report := RunReport{
		RunParams:        RunParams{Runner: RunnerCodexExec, PromptVersion: PromptVersion, Model: CodexExecModel, ReportFormat: "markdown"},
		Counts:           RunCounts{Selected: 2, Dispatched: 2, Enriched: 1, JSONFailed: 1},
		PostingSummaries: []PostingSummary{{PostingID: 7, Title: "PM", Outcome: OutcomeEnriched}},
	}

	var buf bytes.Buffer
	if err := EmitReport(&buf, report, "markdown"); err != nil {
		t.Fatalf("EmitReport: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatal("markdown output is empty")
	}
	if !strings.Contains(out, "# batch-enrich run report") {
		t.Errorf("markdown output missing heading; got:\n%s", out)
	}
	if !strings.Contains(out, "| runner | codex-exec |") {
		t.Errorf("markdown output missing runner parameter; got:\n%s", out)
	}
}

// TestAppendFailures_EmptyOnNoFailures verifies that AppendFailures returns
// no error and creates no file when results contain only enriched and
// db_failed outcomes (neither belongs in failures.jsonl).
func TestAppendFailures_EmptyOnNoFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failures.jsonl")
	setFailuresFilePath(t, path)

	results := []PostingResult{
		{PostingID: 1, Outcome: OutcomeEnriched, Attempts: 1},
		{PostingID: 2, Outcome: OutcomeDBFailed, LastReason: "timeout", Attempts: 1},
	}

	if err := AppendFailures(results, "2026-05-13T00:00:00Z", minimalCfg()); err != nil {
		t.Fatalf("AppendFailures: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no file to be created, but %s exists", path)
	}
}
