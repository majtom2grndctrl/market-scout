// Dispatch loop for batch-enrich: fans one wave of postings out to parallel
// Haiku agents bounded by cfg.MaxParallelAgents, and runs the parse →
// validate → retry cycle per posting. Wave slicing and the per-wave taxonomy
// reload live in main.go; writeback implementation lives in writeback.go —
// see the wave loop in main.go for why dispatch/writeback/reload must
// interleave (newly minted canonical roles, specializations, and skills from
// wave N must be visible to wave N+1's prompts).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PostingResult is the per-posting outcome of one dispatch wave. Outcome,
// LastReason, and LastRawResponse drive failures.jsonl; Classification is
// non-nil only when Outcome is OutcomeEnriched.
type PostingResult struct {
	PostingID       int64
	CompanyID       int64
	Title           string
	Outcome         Outcome
	Summary         string // from AgentResponse, not persisted
	LastReason      string // last validation failure reason or JSON error
	LastRawResponse string // raw subprocess stdout from last attempt
	Attempts        int    // total agent calls made for this posting
	Classification  *ValidatedClassification
}

// agentRunner abstracts subprocess invocation so tests can substitute a fake.
// systemPrompt carries the cache-friendly taxonomy + contract block (stable
// across a wave); userPrompt carries the per-posting message plus any retry
// guidance appended on the current attempt.
type agentRunner interface {
	Run(ctx context.Context, systemPrompt, userPrompt string) (agentText string, rawStdout string, err error)
}

// errAgentFailure is the sentinel returned by a runner when the subprocess
// reports a non-zero exit or the envelope's is_error flag is set. Callers
// treat it the same as a JSON parse failure — agentText is empty and the
// retry path consumes rawStdout for failures.jsonl.
var errAgentFailure = errors.New("agent subprocess reported failure")

// hintReturnJSONOnly is the retry hint used when the agent's response cannot
// be read or parsed as JSON — whether due to a subprocess error or a
// json.Unmarshal failure.
const hintReturnJSONOnly = "Previous response could not be parsed as JSON. Return the schema-conformant JSON object only — no prose, no code fences."

// hintMissingWrapper is the retry hint used when a response parsed as JSON
// but the posting was absent from the `{"results": [...]}` envelope — the
// typical symptom of the agent emitting a bare single-posting object. Tells
// the agent exactly what was missing so it can re-emit with the wrapper.
const hintMissingWrapper = "Your response was not wrapped in {\"results\": [...]}. Every response must use the envelope {\"results\": [{\"posting_id\": N, ...}]} — even for a single posting. Re-emit with the wrapper."

// reasonCancelled is the LastReason value stamped by RunWave on goroutines
// that exit early due to context cancellation before acquiring a semaphore
// slot. BuildReport and AppendFailures match this sentinel to suppress
// cancelled postings from json_failed counts and the failures log.
const reasonCancelled = "cancelled"

// claudeEnvelope is the outer JSON shape returned by `claude -p --output-format json`.
// The classifier's actual JSON lives inside Result as a string.
type claudeEnvelope struct {
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// claudeRunner is the default agentRunner: it shells out to the `claude`
// CLI with the configured model and pipes the prompt over stdin.
type claudeRunner struct {
	model   string
	timeout time.Duration
}

// newClaudeRunner returns the production agentRunner. A zero timeout disables
// the per-call cap — Run will pass the caller's context through unwrapped.
func newClaudeRunner(model string, timeout time.Duration) agentRunner {
	return &claudeRunner{model: model, timeout: timeout}
}

// Run invokes `claude -p --output-format json --model <model> --system-prompt
// <systemPrompt>`, pipes the user prompt over stdin, captures stdout, and
// unwraps the envelope. The split keeps taxonomy + contract in --system-prompt
// (cache-friendly across a wave) and the per-posting body on stdin.
//
// On non-zero exit or is_error=true the raw stdout is returned alongside
// errAgentFailure so callers can log it.
//
// errors.Is(err, context.Canceled) remains true through this wrap chain —
// classifyBatch relies on it to detect cancellation without charging an attempt.
func (r *claudeRunner) Run(ctx context.Context, systemPrompt, userPrompt string) (string, string, error) {
	// Per-call timeout when configured; otherwise the caller's ctx flows through
	// unwrapped so we don't allocate a derived context per invocation.
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "claude", "-p", "--output-format", "json", "--model", r.model, "--system-prompt", systemPrompt)
	cmd.Stdin = bytes.NewReader([]byte(userPrompt))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	raw := stdout.String()

	if runErr != nil {
		// errors.Join preserves both sentinels so errors.Is(err, context.Canceled)
		// remains true when the subprocess was cancelled.
		joined := errors.Join(errAgentFailure, runErr)
		if s := stderr.String(); s != "" {
			return "", raw, fmt.Errorf("%w: stderr: %s", joined, s)
		}
		return "", raw, joined
	}

	var env claudeEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return "", raw, fmt.Errorf("%w: parsing envelope: %v", errAgentFailure, err)
	}
	if env.IsError {
		return "", raw, fmt.Errorf("%w: is_error=true", errAgentFailure)
	}
	return env.Result, raw, nil
}

// RunWave dispatches a wave of postings as batches of cfg.BatchSize, each
// batch run as a single agent call. cfg.MaxParallelAgents bounds concurrent
// batched calls (so effective posting concurrency is MaxParallelAgents ×
// BatchSize). Per-posting retries are always single-posting (batch size 1)
// with a budget of cfg.MaxRetries + 1 total calls including the initial
// batched call. The batched call counts as attempt 1; MaxRetries bounds the
// per-posting single-posting calls that follow. Returns one PostingResult per
// posting in the original wave order. Does not perform writeback.
func RunWave(ctx context.Context, wave []SelectedPosting, taxonomy Taxonomy, cfg Config, runner agentRunner, tracker *ProgressTracker) []PostingResult {
	results := make([]PostingResult, len(wave))
	if len(wave) == 0 {
		return results
	}

	slots := cfg.MaxParallelAgents
	if slots < 1 {
		slots = 1
	}
	sem := make(chan struct{}, slots)

	batchSize := cfg.BatchSize
	if batchSize < 1 {
		batchSize = 1
	}

	// Hoist system prompt: stable across the wave so the prompt cache stays
	// warm and we don't re-render it per batch or per posting.
	systemPrompt := RenderSystemPrompt(taxonomy, cfg.Focus)

	var wg sync.WaitGroup
	for start := 0; start < len(wave); start += batchSize {
		end := start + batchSize
		if end > len(wave) {
			end = len(wave)
		}
		batch := wave[start:end]

		wg.Add(1)
		go func(offset int, batch []SelectedPosting) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					for i, p := range batch {
						// Only stamp slots that weren't already populated by
						// classifyBatch before the panic.
						if results[offset+i].PostingID == 0 {
							results[offset+i] = PostingResult{
								PostingID:  p.PostingID,
								CompanyID:  p.CompanyID,
								Title:      p.Title,
								Outcome:    OutcomeJSONFailed,
								LastReason: fmt.Sprintf("panic: %v", r),
							}
						}
					}
				}
			}()

			// Acquire semaphore slot — bail out if shutdown beat us to it.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				// Order matters: register BatchFinished AFTER semaphore
				// acquisition so LIFO ordering fires it before the panic
				// recovery defer (registered above). Cancelled batches that
				// never acquire a slot don't call BatchStarted, so they must
				// not call BatchFinished either.
				tracker.BatchStarted()
				defer tracker.BatchFinished()
			case <-ctx.Done():
				// OutcomeJSONFailed is reused for cancelled goroutines (no separate
				// OutcomeCancelled exists). Cancelled goroutines never reached the agent,
				// so their LastReason is set to reasonCancelled — AppendFailures and
				// BuildReport filter on that sentinel to exclude them from counts and
				// the failures log.
				for i, p := range batch {
					results[offset+i] = PostingResult{
						PostingID:  p.PostingID,
						CompanyID:  p.CompanyID,
						Title:      p.Title,
						Outcome:    OutcomeJSONFailed,
						LastReason: reasonCancelled,
					}
				}
				return
			}

			batchResults := classifyBatch(ctx, batch, taxonomy, cfg, runner, systemPrompt)
			for i, r := range batchResults {
				results[offset+i] = r
			}
		}(start, batch)
	}
	wg.Wait()
	return results
}

// stripCodeFence removes markdown code fences that the claude CLI sometimes
// wraps around JSON output despite being asked for raw JSON. It strips the
// opening fence line (e.g. "```json" or "```") and the closing "```", then
// trims surrounding whitespace. Input without a fence passes through unchanged.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Strip opening fence line (e.g. "```json" or "```").
	if idx := strings.Index(s, "\n"); idx != -1 {
		s = s[idx+1:]
	}
	// Strip closing fence.
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

// classifyBatch runs one batched agent call across the supplied postings, then
// drops to single-posting retries for any posting that failed validation, came
// back missing from the response, or was lost to a runner/parse failure.
//
// The returned slice is ordered to match the input postings slice.
//
// Attempts accounting: a single completed runner.Run increments Attempts by 1
// for every posting it addressed. Retries each charge 1 to the single posting
// they target. Cancellation never charges an attempt.
func classifyBatch(
	ctx context.Context,
	postings []SelectedPosting,
	taxonomy Taxonomy,
	cfg Config,
	runner agentRunner,
	systemPrompt string,
) []PostingResult {
	results := make([]PostingResult, len(postings))
	for i, p := range postings {
		results[i] = PostingResult{
			PostingID: p.PostingID,
			CompanyID: p.CompanyID,
			Title:     p.Title,
		}
	}

	// idxOf maps PostingID → index in results so retry bookkeeping can find
	// the slot regardless of input order.
	idxOf := make(map[int64]int, len(postings))
	expectedIDs := make([]int64, len(postings))
	for i, p := range postings {
		idxOf[p.PostingID] = i
		expectedIDs[i] = p.PostingID
	}

	// retryQueue tracks postings that need single-posting follow-up after the
	// batched call. Hints carry forward the most recent validator guidance (or
	// hintReturnJSONOnly for parse/runner failures).
	type retryItem struct {
		posting SelectedPosting
		hints   []string
	}
	var retryQueue []retryItem

	// Bail out cleanly if cancellation beat us to the batched call. Every
	// posting in this batch reports reasonCancelled with zero attempts.
	if err := ctx.Err(); err != nil {
		for i := range results {
			results[i].Outcome = OutcomeJSONFailed
			results[i].LastReason = reasonCancelled
		}
		return results
	}

	userMessage := RenderBatchedUserMessage(postings)
	agentText, rawStdout, runErr := runner.Run(ctx, systemPrompt, userMessage)

	switch {
	case runErr != nil && (ctx.Err() != nil || errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)):
		// Cancellation during the batched call: never reached a usable result.
		// Don't increment Attempts; stamp cancelled and stop.
		for i := range results {
			results[i].Outcome = OutcomeJSONFailed
			results[i].LastReason = reasonCancelled
		}
		return results

	case runErr != nil:
		// Non-cancellation runner failure: charge 1 attempt to each posting in
		// the batch and queue every posting for single-posting retry with the
		// generic "return JSON only" hint.
		for i, p := range postings {
			results[i].Attempts++
			results[i].LastRawResponse = rawStdout
			results[i].LastReason = runErr.Error()
			retryQueue = append(retryQueue, retryItem{posting: p, hints: []string{hintReturnJSONOnly}})
		}

	default:
		// Runner succeeded. Parse the batched envelope.
		parsed, _, parseErr := ParseBatchedResponse(stripCodeFence(agentText), expectedIDs)
		if parseErr != nil {
			// JSON parse failure on the batched envelope: same treatment as
			// runner failure — charge 1 to each posting, retry each as single.
			for i, p := range postings {
				results[i].Attempts++
				results[i].LastRawResponse = rawStdout
				results[i].LastReason = fmt.Sprintf("json parse: %v", parseErr)
				retryQueue = append(retryQueue, retryItem{posting: p, hints: []string{hintReturnJSONOnly}})
			}
		} else {
			// Per-posting routing from the parsed batch response.
			for _, p := range postings {
				idx := idxOf[p.PostingID]
				results[idx].LastRawResponse = rawStdout
				results[idx].Attempts++

				resp, present := parsed[p.PostingID]
				if !present {
					// Missing from response: charge the batched attempt and
					// queue for single-posting retry with the generic hint.
					results[idx].LastReason = "missing from batched response"
					retryQueue = append(retryQueue, retryItem{posting: p, hints: []string{hintMissingWrapper}})
					continue
				}

				validated, vErr := Validate(resp, taxonomy)
				if vErr == nil {
					results[idx].Outcome = OutcomeEnriched
					results[idx].Summary = resp.Summary
					results[idx].LastReason = ""
					results[idx].Classification = &validated
					continue
				}

				var vf ValidationFailure
				if errors.As(vErr, &vf) {
					results[idx].LastReason = vf.Error()
					results[idx].Summary = resp.Summary
					// Seed OutcomeValidationFailed: the batched call produced a
					// parseable response, so terminal classification should reflect
					// that even if the retry loop never runs (e.g. MaxRetries=0) or
					// is cancelled before the first retry completes. A successful
					// retry below overwrites this to OutcomeEnriched.
					results[idx].Outcome = OutcomeValidationFailed
					retryQueue = append(retryQueue, retryItem{posting: p, hints: vf.Hints})
					continue
				}

				// Contract regression: Validate returned a non-ValidationFailure
				// error. Treat as parse miss and retry with the generic hint —
				// never forward a raw Go error string to the agent.
				results[idx].LastReason = vErr.Error()
				retryQueue = append(retryQueue, retryItem{posting: p, hints: []string{hintReturnJSONOnly}})
			}
		}
	}

	// Single-posting retry loop. Budget per posting is cfg.MaxRetries + 1
	// total calls including the initial batched call (already charged above).
	totalBudget := cfg.MaxRetries + 1
	for _, item := range retryQueue {
		idx := idxOf[item.posting.PostingID]
		hints := item.hints
		// lastParsed tracks whether we've ever seen a parseable response for
		// this posting during the retry loop, so terminal classification can
		// distinguish OutcomeJSONFailed (never parsed) from
		// OutcomeValidationFailed (parsed but never validated).
		var lastParsed *AgentResponse

		baseUserMessage := RenderBatchedUserMessage([]SelectedPosting{item.posting})

		for results[idx].Attempts < totalBudget {
			if err := ctx.Err(); err != nil {
				// Cancelled mid-retry: preserve prior outcome (set when entering
				// retryQueue), stamp LastReason only.
				results[idx].LastReason = reasonCancelled
				break
			}

			retryUserMessage := RenderRetryPrompt(baseUserMessage, hints)
			retryText, retryRaw, retryErr := runner.Run(ctx, systemPrompt, retryUserMessage)

			if retryErr != nil {
				if ctx.Err() != nil || errors.Is(retryErr, context.Canceled) || errors.Is(retryErr, context.DeadlineExceeded) {
					// Cancelled mid-retry: preserve prior outcome (set when entering
					// retryQueue), stamp LastReason only. Don't charge this call.
					results[idx].LastReason = reasonCancelled
					break
				}
				results[idx].Attempts++
				results[idx].LastRawResponse = retryRaw
				results[idx].LastReason = retryErr.Error()
				hints = []string{hintReturnJSONOnly}
				// Do not reset lastParsed: it tracks whether we have *ever* seen
				// a parseable response. A runner error on a later attempt must
				// not erase the fact that an earlier attempt parsed cleanly.
				continue
			}

			results[idx].Attempts++
			results[idx].LastRawResponse = retryRaw

			parsedMap, _, parseErr := ParseBatchedResponse(stripCodeFence(retryText), []int64{item.posting.PostingID})
			if parseErr != nil {
				results[idx].LastReason = fmt.Sprintf("json parse: %v", parseErr)
				hints = []string{hintReturnJSONOnly}
				// Preserve lastParsed: see runner-error branch above.
				continue
			}
			resp, present := parsedMap[item.posting.PostingID]
			if !present {
				results[idx].LastReason = "missing from batched response"
				hints = []string{hintMissingWrapper}
				// Preserve lastParsed: see runner-error branch above.
				continue
			}
			lastParsed = &resp

			validated, vErr := Validate(resp, taxonomy)
			if vErr == nil {
				results[idx].Outcome = OutcomeEnriched
				results[idx].Summary = resp.Summary
				results[idx].LastReason = ""
				results[idx].Classification = &validated
				break
			}

			var vf ValidationFailure
			if errors.As(vErr, &vf) {
				results[idx].LastReason = vf.Error()
				results[idx].Summary = resp.Summary
				hints = vf.Hints
				continue
			}

			// Contract regression — fall back to the generic hint.
			results[idx].LastReason = vErr.Error()
			hints = []string{hintReturnJSONOnly}
		}

		// Terminal classification for this posting. Skip if we already landed
		// on OutcomeEnriched, or if Outcome was seeded to OutcomeValidationFailed
		// when the posting entered retryQueue from the batched call (a parseable
		// but invalid response). The seeded outcome is correct even if the
		// retry loop never ran (MaxRetries=0) or was cancelled before completing.
		if results[idx].Outcome == OutcomeEnriched || results[idx].Outcome == OutcomeValidationFailed {
			if results[idx].Outcome == OutcomeValidationFailed && lastParsed != nil {
				results[idx].Summary = lastParsed.Summary
			}
			continue
		}
		if lastParsed == nil {
			results[idx].Outcome = OutcomeJSONFailed
		} else {
			results[idx].Outcome = OutcomeValidationFailed
			results[idx].Summary = lastParsed.Summary
		}
	}

	return results
}
