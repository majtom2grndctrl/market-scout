package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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
	responder func(attempt int, systemPrompt, userPrompt string) (agentText string, rawStdout string, err error)
}

func (f *fakeRunner) Run(_ context.Context, systemPrompt, userPrompt string) (string, string, error) {
	n := atomic.AddInt32(&f.calls, 1)
	return f.responder(int(n), systemPrompt, userPrompt)
}

// validResponseJSON marshals validResponse (from validate_test.go) wrapped in
// the batched {"results":[...]} envelope so the dispatch tests can hand a
// real, schema-conformant payload back to the orchestrator.
func validResponseJSON(t *testing.T) string {
	t.Helper()
	return wrapBatched(t, validResponse())
}

// wrapBatched wraps one or more AgentResponse values in the batched
// {"results":[...]} envelope. Dispatch parses batched responses via
// ParseBatchedResponse, so test payloads must use this shape even for
// single-posting waves.
func wrapBatched(t *testing.T, responses ...AgentResponse) string {
	t.Helper()
	raw, err := json.Marshal(BatchedAgentResponse{Results: responses})
	if err != nil {
		t.Fatalf("marshal batched response: %v", err)
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
		BatchSize:         1,
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
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return resp, resp, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)

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
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return "", "raw subprocess output", errors.New("subprocess exploded")
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
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
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			// Non-JSON text — parse will fail every attempt.
			return "not json at all", "envelope raw", nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
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
	rawStr := wrapBatched(t, resp)

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return rawStr, rawStr, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
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

// TestRunWave_BatchValidationFailed_MaxRetriesZero verifies the terminal
// classification when the batched call returns a parseable-but-invalid
// response and MaxRetries=0 (totalBudget=1, no retry headroom). The seeded
// OutcomeValidationFailed from the batched routing must survive: previously
// the terminal block would mistakenly stamp OutcomeJSONFailed because the
// retry-loop's local lastParsed was nil.
func TestRunWave_BatchValidationFailed_MaxRetriesZero(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.MaxRetries = 0 // totalBudget=1; only the batched call is allowed.

	bad := validResponse()
	bad.CanonicalRoles[0].Slug = "Bad_Slug" // parses cleanly but fails Validate.
	rawStr := wrapBatched(t, bad)

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return rawStr, rawStr, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
	r := results[0]

	if r.Outcome != OutcomeValidationFailed {
		t.Errorf("outcome: want %q, got %q (reason=%q)", OutcomeValidationFailed, r.Outcome, r.LastReason)
	}
	if r.Attempts != 1 {
		t.Errorf("attempts: want 1, got %d", r.Attempts)
	}
}

// TestRunWave_ContextCancellation verifies the invariant that matters under
// cancellation: no posting lands OutcomeEnriched and every posting carries
// reasonCancelled. We can't assert the runner is never invoked — the
// semaphore-acquire select has two ready cases when ctx is already cancelled
// and Go picks pseudo-randomly, so a goroutine may legitimately call the
// runner before the cancel branch wins. The runner returns a benign response
// in that case; the key check is the terminal outcome and reason.
func TestRunWave_ContextCancellation(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.MaxParallelAgents = 1

	resp := validResponseJSON(t)
	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return resp, resp, nil
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

	results := RunWave(ctx, wave, tax, cfg, runner, nil)

	if len(results) != len(wave) {
		t.Fatalf("want %d results, got %d", len(wave), len(results))
	}
	for i, r := range results {
		if r.Outcome == OutcomeEnriched {
			t.Errorf("result[%d]: posting should not enrich under a cancelled context (got reason=%q)",
				i, r.LastReason)
		}
		if r.LastReason != reasonCancelled {
			t.Errorf("result[%d]: want LastReason=%q, got %q (outcome=%q)",
				i, reasonCancelled, r.LastReason, r.Outcome)
		}
	}
}

func TestRunWave_RetryThenSucceed_Enriched(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	good := validResponseJSON(t)

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			if attempt == 1 {
				return "garbage", "garbage", nil
			}
			return good, good, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
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
// makeBatchPostings returns n SelectedPostings with sequential PostingIDs
// starting at 1, matching the slug "software-engineer" in newTestTaxonomy()
// so canned responses cleanly validate.
func makeBatchPostings(n int) []SelectedPosting {
	postings := make([]SelectedPosting, n)
	for i := 0; i < n; i++ {
		postings[i] = SelectedPosting{
			PostingID:       int64(i + 1),
			CompanyID:       100,
			Title:           "Software Engineer",
			DescriptionText: "We need an engineer.",
		}
	}
	return postings
}

// validResponseFor builds a validResponse() with PostingID overridden so the
// batched envelope can carry one entry per posting in the wave.
func validResponseFor(postingID int64) AgentResponse {
	r := validResponse()
	r.PostingID = postingID
	return r
}

// TestRunWave_BatchAllSucceed verifies the happy-path batched call: a single
// runner invocation covers every posting in the batch, each posting lands
// OutcomeEnriched with Attempts==1.
func TestRunWave_BatchAllSucceed(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.BatchSize = 3
	cfg.WaveSize = 3

	postings := makeBatchPostings(3)
	resp := wrapBatched(t, validResponseFor(1), validResponseFor(2), validResponseFor(3))

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			return resp, resp, nil
		},
	}

	results := RunWave(context.Background(), postings, tax, cfg, runner, nil)

	if got := atomic.LoadInt32(&runner.calls); got != 1 {
		t.Errorf("expected 1 runner call, got %d", got)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Outcome != OutcomeEnriched {
			t.Errorf("result[%d]: want %q, got %q (reason=%q)", i, OutcomeEnriched, r.Outcome, r.LastReason)
		}
		if r.Attempts != 1 {
			t.Errorf("result[%d]: want Attempts=1, got %d", i, r.Attempts)
		}
		if r.Classification == nil {
			t.Errorf("result[%d]: expected non-nil Classification", i)
		}
	}
}

// TestRunWave_BatchOneFailsValidation verifies that when one posting in the
// batched response fails Phase A validation, only that posting enters the
// single-posting retry path. The retry succeeds on the first try, so the
// retried posting lands OutcomeEnriched with Attempts==2; the others stay at
// Attempts==1.
func TestRunWave_BatchOneFailsValidation(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.BatchSize = 3
	cfg.WaveSize = 3

	postings := makeBatchPostings(3)

	good1 := validResponseFor(1)
	bad2 := validResponseFor(2)
	bad2.CanonicalRoles[0].Slug = "Bad_Slug" // parses but fails validation
	good3 := validResponseFor(3)
	batchedResp := wrapBatched(t, good1, bad2, good3)

	// Single-posting retry payload for posting 2.
	retryResp := wrapBatched(t, validResponseFor(2))

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			if attempt == 1 {
				return batchedResp, batchedResp, nil
			}
			return retryResp, retryResp, nil
		},
	}

	results := RunWave(context.Background(), postings, tax, cfg, runner, nil)

	if got := atomic.LoadInt32(&runner.calls); got != 2 {
		t.Errorf("expected 2 runner calls (1 batched + 1 retry), got %d", got)
	}
	for i, r := range results {
		if r.Outcome != OutcomeEnriched {
			t.Errorf("result[%d] (posting %d): want %q, got %q (reason=%q)",
				i, r.PostingID, OutcomeEnriched, r.Outcome, r.LastReason)
		}
	}
	// Posting 1 and 3 succeeded on the batched call; posting 2 needed a retry.
	if a := results[0].Attempts; a != 1 {
		t.Errorf("posting 1: want Attempts=1, got %d", a)
	}
	if a := results[1].Attempts; a != 2 {
		t.Errorf("posting 2 (retried): want Attempts=2, got %d", a)
	}
	if a := results[2].Attempts; a != 1 {
		t.Errorf("posting 3: want Attempts=1, got %d", a)
	}
}

// TestRunWave_BatchMissingPostingID verifies that when the batched response
// omits one posting, that posting alone enters the single-posting retry
// path. The other postings are unaffected.
func TestRunWave_BatchMissingPostingID(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.BatchSize = 3
	cfg.WaveSize = 3

	postings := makeBatchPostings(3)

	// Posting 2 missing from the batched response.
	batchedResp := wrapBatched(t, validResponseFor(1), validResponseFor(3))
	retryResp := wrapBatched(t, validResponseFor(2))

	var retryUserPrompts []string
	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			if attempt == 1 {
				return batchedResp, batchedResp, nil
			}
			retryUserPrompts = append(retryUserPrompts, userPrompt)
			return retryResp, retryResp, nil
		},
	}

	results := RunWave(context.Background(), postings, tax, cfg, runner, nil)

	if got := atomic.LoadInt32(&runner.calls); got != 2 {
		t.Errorf("expected 2 runner calls (1 batched + 1 retry for missing posting), got %d", got)
	}
	if len(retryUserPrompts) != 1 {
		t.Fatalf("expected 1 retry prompt, got %d", len(retryUserPrompts))
	}
	// The retry prompt should target posting 2 only.
	if !strings.Contains(retryUserPrompts[0], "# Posting 2:") {
		t.Errorf("retry prompt should target posting 2, got: %s", retryUserPrompts[0])
	}
	if strings.Contains(retryUserPrompts[0], "# Posting 1:") || strings.Contains(retryUserPrompts[0], "# Posting 3:") {
		t.Errorf("retry prompt should not include postings 1 or 3, got: %s", retryUserPrompts[0])
	}

	for i, r := range results {
		if r.Outcome != OutcomeEnriched {
			t.Errorf("result[%d] (posting %d): want %q, got %q (reason=%q)",
				i, r.PostingID, OutcomeEnriched, r.Outcome, r.LastReason)
		}
	}
	if a := results[0].Attempts; a != 1 {
		t.Errorf("posting 1: want Attempts=1, got %d", a)
	}
	if a := results[1].Attempts; a != 2 {
		t.Errorf("posting 2 (retried): want Attempts=2, got %d", a)
	}
	if a := results[2].Attempts; a != 1 {
		t.Errorf("posting 3: want Attempts=1, got %d", a)
	}
}

// TestRunWave_BatchJSONParseFailureFansOutToSingleRetries verifies that when
// the batched call returns unparseable JSON, every posting in the batch enters
// the single-posting retry path. With a clean retry response, each posting
// lands OutcomeEnriched with Attempts==2.
func TestRunWave_BatchJSONParseFailureFansOutToSingleRetries(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.BatchSize = 3
	cfg.WaveSize = 3
	cfg.MaxParallelAgents = 1 // serialise retries so call accounting is deterministic

	postings := makeBatchPostings(3)

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			if attempt == 1 {
				// Garbage on the batched call.
				return "not json", "not json", nil
			}
			// Each retry targets a single posting; figure out which by parsing
			// the heading we rendered, and respond with the matching JSON.
			for _, p := range postings {
				marker := "# Posting " + itoa(p.PostingID) + ":"
				if strings.Contains(userPrompt, marker) {
					payload := wrapBatched(t, validResponseFor(p.PostingID))
					return payload, payload, nil
				}
			}
			t.Errorf("retry prompt did not match any expected posting heading: %s", userPrompt)
			return "", "", nil
		},
	}

	results := RunWave(context.Background(), postings, tax, cfg, runner, nil)

	// 1 batched call + 3 single-posting retries.
	if got := atomic.LoadInt32(&runner.calls); got != 4 {
		t.Errorf("expected 4 runner calls (1 batched + 3 retries), got %d", got)
	}
	for i, r := range results {
		if r.Outcome != OutcomeEnriched {
			t.Errorf("result[%d] (posting %d): want %q, got %q (reason=%q)",
				i, r.PostingID, OutcomeEnriched, r.Outcome, r.LastReason)
		}
		if r.Attempts != 2 {
			t.Errorf("result[%d] (posting %d): want Attempts=2, got %d", i, r.PostingID, r.Attempts)
		}
	}
}

// TestRunWave_TwoBatchesHappyPath verifies that a wave of 2N postings with
// BatchSize N produces exactly 2 batched runner calls on the happy path.
func TestRunWave_TwoBatchesHappyPath(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.BatchSize = 2
	cfg.WaveSize = 4
	cfg.MaxParallelAgents = 2

	postings := makeBatchPostings(4)

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			// Inspect the user prompt to figure out which postings this batch
			// addressed, and respond with matching JSON. This decouples the
			// response from goroutine scheduling order.
			var responses []AgentResponse
			for _, p := range postings {
				marker := "# Posting " + itoa(p.PostingID) + ":"
				if strings.Contains(userPrompt, marker) {
					responses = append(responses, validResponseFor(p.PostingID))
				}
			}
			payload := wrapBatched(t, responses...)
			return payload, payload, nil
		},
	}

	results := RunWave(context.Background(), postings, tax, cfg, runner, nil)

	if got := atomic.LoadInt32(&runner.calls); got != 2 {
		t.Errorf("expected 2 batched runner calls, got %d", got)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Outcome != OutcomeEnriched {
			t.Errorf("result[%d] (posting %d): want %q, got %q (reason=%q)",
				i, r.PostingID, OutcomeEnriched, r.Outcome, r.LastReason)
		}
		if r.Attempts != 1 {
			t.Errorf("result[%d] (posting %d): want Attempts=1, got %d", i, r.PostingID, r.Attempts)
		}
	}
}

// TestRunWave_BatchSizeOne verifies the regression case: with BatchSize==1,
// a wave of N postings produces N runner calls (one per posting) on the happy
// path.
func TestRunWave_BatchSizeOne(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()
	cfg.BatchSize = 1
	cfg.WaveSize = 3
	cfg.MaxParallelAgents = 1

	postings := makeBatchPostings(3)

	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			for _, p := range postings {
				marker := "# Posting " + itoa(p.PostingID) + ":"
				if strings.Contains(userPrompt, marker) {
					payload := wrapBatched(t, validResponseFor(p.PostingID))
					return payload, payload, nil
				}
			}
			t.Errorf("prompt did not match any expected posting heading: %s", userPrompt)
			return "", "", nil
		},
	}

	results := RunWave(context.Background(), postings, tax, cfg, runner, nil)

	if got := atomic.LoadInt32(&runner.calls); got != 3 {
		t.Errorf("expected 3 runner calls (one per posting), got %d", got)
	}
	for i, r := range results {
		if r.Outcome != OutcomeEnriched {
			t.Errorf("result[%d] (posting %d): want %q, got %q (reason=%q)",
				i, r.PostingID, OutcomeEnriched, r.Outcome, r.LastReason)
		}
		if r.Attempts != 1 {
			t.Errorf("result[%d] (posting %d): want Attempts=1, got %d", i, r.PostingID, r.Attempts)
		}
	}
}

// itoa is a small int64→string helper used by batch tests to build the
// "# Posting <id>:" heading marker they grep for in user prompts.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestRunWave_ValidationFailThenSucceed_Enriched(t *testing.T) {
	tax := newTestTaxonomy()
	cfg := dispatchTestConfig()

	// Build a response that parses fine but fails slug validation.
	// "bad slug!" contains a space and exclamation mark, both disallowed by
	// slugPattern, so Validate rejects it with a FailInvalidSlug hint.
	bad := validResponse()
	bad.CanonicalRoles[0].Slug = "bad slug!"
	badJSON := wrapBatched(t, bad)

	good := validResponseJSON(t)

	var secondPrompt string
	runner := &fakeRunner{
		responder: func(attempt int, systemPrompt, userPrompt string) (string, string, error) {
			if attempt == 1 {
				return badJSON, badJSON, nil
			}
			secondPrompt = userPrompt
			return good, good, nil
		},
	}

	results := RunWave(context.Background(), []SelectedPosting{dispatchTestPosting()}, tax, cfg, runner, nil)
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
