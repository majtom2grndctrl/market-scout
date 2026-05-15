package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeRunner is a programmable agentRunner for dispatch loop tests. The
// responder closure is called once per attempt and decides what to return
// based on the call index, so tests can model "first attempt fails, second
// succeeds" sequences without coordinating shared state across goroutines.
type fakeRunner struct {
	calls     int32
	responder func(attempt int, prompt string) (agentText string, rawStdout string, err error)
}

func (f *fakeRunner) Run(_ context.Context, prompt string) (string, string, error) {
	n := atomic.AddInt32(&f.calls, 1)
	return f.responder(int(n), prompt)
}

// validResponseJSON marshals validResponse (from validate_test.go) so the
// dispatch tests can hand a real, schema-conformant payload back to the
// orchestrator.
func validResponseJSON(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(validResponse())
	if err != nil {
		t.Fatalf("marshal valid response: %v", err)
	}
	return string(raw)
}

// dispatchTestPosting matches the slug "software-engineer" so the response
// from validResponse() validates against newTestTaxonomy().
func dispatchTestPosting() SelectedPosting {
	return SelectedPosting{
		PostingID:       1,
		CompanyID:       100,
		Title:           "Software Engineer",
		DescriptionText: "We need an engineer.",
	}
}

func dispatchTestConfig() Config {
	return Config{
		PromptVersion:     PromptVersion,
		Model:             Model,
		WaveSize:          1,
		MaxRetries:        2, // 3 total attempts
		MaxParallelAgents: 1,
		ReportFormat:      "json",
	}
}

func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "raw JSON passes through unchanged",
			input: `{"roles":["engineer"]}`,
			want:  `{"roles":["engineer"]}`,
		},
		{
			name:  "json-tagged fence stripped",
			input: "```json\n{\"roles\":[\"engineer\"]}\n```",
			want:  `{"roles":["engineer"]}`,
		},
		{
			name:  "untagged fence stripped",
			input: "```\n{\"roles\":[\"engineer\"]}\n```",
			want:  `{"roles":["engineer"]}`,
		},
		{
			name:  "surrounding whitespace trimmed",
			input: "  \n```json\n{\"k\":\"v\"}\n```\n  ",
			want:  `{"k":"v"}`,
		},
		{
			name:  "nested backticks in JSON values not affected",
			input: "```json\n{\"note\":\"use ```go``` here\"}\n```",
			want:  "{\"note\":\"use ```go``` here\"}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCodeFence(tc.input)
			if got != tc.want {
				t.Errorf("stripCodeFence(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRunWave_FirstAttemptSuccess_Enriched(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	resp := validResponseJSON(t)

	runner := &fakeRunner{
		responder: func(attempt int, prompt string) (string, string, error) {
			return resp, resp, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Outcome != OutcomeEnriched {
		t.Errorf("outcome: want %q, got %q (reason=%q)", OutcomeEnriched, r.Outcome, r.LastReason)
	}
	if r.Attempts != 1 {
		t.Errorf("attempts: want 1, got %d", r.Attempts)
	}
	if r.Classification == nil {
		t.Errorf("expected non-nil Classification on enriched result")
	}
}

func TestRunWave_AlwaysError_JSONFailed(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()

	runner := &fakeRunner{
		responder: func(attempt int, prompt string) (string, string, error) {
			return "", "raw subprocess output", errors.New("subprocess exploded")
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner)
	r := results[0]

	if r.Outcome != OutcomeJSONFailed {
		t.Errorf("outcome: want %q, got %q", OutcomeJSONFailed, r.Outcome)
	}
	wantAttempts := cfg.MaxRetries + 1
	if r.Attempts != wantAttempts {
		t.Errorf("attempts: want %d, got %d", wantAttempts, r.Attempts)
	}
	if r.LastRawResponse != "raw subprocess output" {
		t.Errorf("expected raw stdout to be captured, got %q", r.LastRawResponse)
	}
}

func TestRunWave_InvalidJSON_JSONFailed(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()

	runner := &fakeRunner{
		responder: func(attempt int, prompt string) (string, string, error) {
			// Non-JSON text — parse will fail every attempt.
			return "not json at all", "envelope raw", nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner)
	r := results[0]

	if r.Outcome != OutcomeJSONFailed {
		t.Errorf("outcome: want %q, got %q", OutcomeJSONFailed, r.Outcome)
	}
	if r.Attempts != cfg.MaxRetries+1 {
		t.Errorf("attempts: want %d, got %d", cfg.MaxRetries+1, r.Attempts)
	}
}

func TestRunWave_ValidationFailure_ValidationFailed(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()

	// Build a response with an invalid slug — it parses fine but Validate
	// rejects it every attempt. The orchestrator should exhaust retries and
	// land on OutcomeValidationFailed (parsed but never validated).
	resp := validResponse()
	resp.CanonicalRoles[0].Slug = "Bad_Slug"
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rawStr := string(raw)

	runner := &fakeRunner{
		responder: func(attempt int, prompt string) (string, string, error) {
			return rawStr, rawStr, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner)
	r := results[0]

	if r.Outcome != OutcomeValidationFailed {
		t.Errorf("outcome: want %q, got %q (reason=%q)", OutcomeValidationFailed, r.Outcome, r.LastReason)
	}
	if r.Attempts != cfg.MaxRetries+1 {
		t.Errorf("attempts: want %d, got %d", cfg.MaxRetries+1, r.Attempts)
	}
	if r.Summary == "" {
		t.Errorf("expected Summary to be carried from last parsed response")
	}
}

// TestRunWave_ContextCancellation verifies that when the parent context is
// already cancelled before RunWave dispatches, every goroutine bails out at
// the semaphore and stamps reasonCancelled. The fake runner panics if invoked,
// which would surface as an OutcomeJSONFailed with a panic reason rather than
// a clean "cancelled" — catching that regression is the point of the test.
func TestRunWave_ContextCancellation(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	// Throttle slot count so the cancel path through the semaphore select
	// is the dominant exit even though all goroutines start concurrently.
	cfg.MaxParallelAgents = 1

	runner := &fakeRunner{
		responder: func(attempt int, prompt string) (string, string, error) {
			t.Errorf("runner should not be invoked after context cancellation")
			return "", "", nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wave := []SelectedPosting{
		{PostingID: 1, CompanyID: 100, Title: "A", DescriptionText: "x"},
		{PostingID: 2, CompanyID: 100, Title: "B", DescriptionText: "x"},
		{PostingID: 3, CompanyID: 100, Title: "C", DescriptionText: "x"},
		{PostingID: 4, CompanyID: 100, Title: "D", DescriptionText: "x"},
	}

	results := RunWave(ctx, wave, tax, cfg, runner)

	if len(results) != len(wave) {
		t.Fatalf("want %d results, got %d", len(wave), len(results))
	}
	for i, r := range results {
		if r.LastReason != reasonCancelled {
			t.Errorf("result[%d]: want LastReason=%q, got %q (outcome=%q)",
				i, reasonCancelled, r.LastReason, r.Outcome)
		}
		if r.Attempts != 0 {
			t.Errorf("result[%d]: cancelled goroutine should have 0 attempts, got %d", i, r.Attempts)
		}
	}
}

func TestRunWave_RetryThenSucceed_Enriched(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	good := validResponseJSON(t)

	runner := &fakeRunner{
		responder: func(attempt int, prompt string) (string, string, error) {
			if attempt == 1 {
				return "garbage", "garbage", nil
			}
			return good, good, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner)
	r := results[0]

	if r.Outcome != OutcomeEnriched {
		t.Errorf("outcome: want %q, got %q (reason=%q)", OutcomeEnriched, r.Outcome, r.LastReason)
	}
	if r.Attempts != 2 {
		t.Errorf("attempts: want 2, got %d", r.Attempts)
	}
}

// TestRunWave_ValidationFailThenSucceed_Enriched exercises the validation-
// recovery path: the first attempt returns well-formed JSON that fails Phase A
// (bad slug format), causing the orchestrator to build a hint and append a
// "Retry guidance" block before re-invoking. The second attempt returns a
// valid response. This is the more delicate retry branch — it exercises
// RenderRetryPrompt and the ValidationFailure hint path, not just the
// JSON-parse fallback.
func TestRunWave_ValidationFailThenSucceed_Enriched(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()

	// Build a response that parses fine but fails slug validation.
	// "bad slug!" contains a space and exclamation mark, both disallowed by
	// slugPattern, so Validate rejects it with a FailInvalidSlug hint.
	bad := validResponse()
	bad.CanonicalRoles[0].Slug = "bad slug!"
	badJSON, err := json.Marshal(bad)
	if err != nil {
		t.Fatalf("marshal bad response: %v", err)
	}

	good := validResponseJSON(t)

	var secondPrompt string
	runner := &fakeRunner{
		responder: func(attempt int, prompt string) (string, string, error) {
			if attempt == 1 {
				return string(badJSON), string(badJSON), nil
			}
			secondPrompt = prompt
			return good, good, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner)
	r := results[0]

	if r.Outcome != OutcomeEnriched {
		t.Errorf("outcome: want %q, got %q (reason=%q)", OutcomeEnriched, r.Outcome, r.LastReason)
	}
	if r.Attempts != 2 {
		t.Errorf("attempts: want 2, got %d", r.Attempts)
	}
	if !strings.Contains(secondPrompt, "Retry guidance") {
		t.Errorf("second prompt should contain %q; got prompt:\n%s", "Retry guidance", secondPrompt)
	}
}
