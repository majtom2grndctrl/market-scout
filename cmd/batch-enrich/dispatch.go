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
// classifyOne relies on it to detect cancellation without re-executing.
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

// RunWave dispatches a wave of postings concurrently, bounded by
// cfg.MaxParallelAgents. Each posting gets up to cfg.MaxRetries + 1
// agent invocations: an initial attempt followed by retry rounds that
// fold validator hints back into the prompt. Returns one PostingResult
// per posting in the original wave order. Does not perform writeback.
func RunWave(ctx context.Context, wave []SelectedPosting, taxonomy Taxonomy, cfg Config, runner agentRunner) []PostingResult {
	results := make([]PostingResult, len(wave))

	slots := cfg.MaxParallelAgents
	if slots < 1 {
		slots = 1
	}
	sem := make(chan struct{}, slots)

	var wg sync.WaitGroup
	for i, posting := range wave {
		wg.Add(1)
		go func(idx int, p SelectedPosting) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = PostingResult{
						PostingID:  p.PostingID,
						CompanyID:  p.CompanyID,
						Title:      p.Title,
						Outcome:    OutcomeJSONFailed,
						LastReason: fmt.Sprintf("panic: %v", r),
					}
				}
			}()

			// Acquire semaphore slot — bail out if shutdown beat us to it.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				// OutcomeJSONFailed is reused for cancelled goroutines (no separate
				// OutcomeCancelled exists). Cancelled goroutines never reached the agent,
				// so their LastReason is set to reasonCancelled — AppendFailures and
				// BuildReport filter on that sentinel to exclude them from counts and
				// the failures log. Adding an OutcomeCancelled variant would require
				// updating those filters; the sentinel approach avoids the new outcome type.
				results[idx] = PostingResult{
					PostingID:  p.PostingID,
					CompanyID:  p.CompanyID,
					Title:      p.Title,
					Outcome:    OutcomeJSONFailed,
					LastReason: reasonCancelled,
				}
				return
			}

			results[idx] = classifyOne(ctx, p, taxonomy, cfg, runner)
		}(i, posting)
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

// classifyOne runs the parse → validate → retry cycle for a single posting.
// The retry loop budget is cfg.MaxRetries additional attempts after the
// initial call, capped at cfg.MaxRetries + 1 total. The function returns
// as soon as validation succeeds or the budget is exhausted.
func classifyOne(ctx context.Context, posting SelectedPosting, taxonomy Taxonomy, cfg Config, runner agentRunner) PostingResult {
	result := PostingResult{
		PostingID: posting.PostingID,
		CompanyID: posting.CompanyID,
		Title:     posting.Title,
	}

	// systemPrompt is built once per posting and reused unchanged across
	// retries so the Claude CLI keeps a warm prompt cache. The userPrompt
	// starts as the bare posting message and gets retry guidance appended
	// each time the agent fails validation or returns unparseable output.
	systemPrompt := RenderSystemPrompt(taxonomy, cfg.Focus)
	originalUserMessage := RenderUserMessage(posting)
	userPrompt := originalUserMessage

	totalAttempts := cfg.MaxRetries + 1
	var lastParsed *AgentResponse

	for attempt := 0; attempt < totalAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			result.Outcome = OutcomeJSONFailed
			result.LastReason = reasonCancelled
			return result
		}

		agentText, rawStdout, runErr := runner.Run(ctx, systemPrompt, userPrompt)

		if runErr != nil {
			// Subprocess interruption via ctx cancellation: don't count the
			// aborted attempt or capture its partial stdout. Exit immediately
			// so the result reflects the cancellation cleanly.
			if ctx.Err() != nil || errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
				result.Outcome = OutcomeJSONFailed
				result.LastReason = reasonCancelled
				return result
			}
			result.Attempts++
			result.LastRawResponse = rawStdout
			result.LastReason = runErr.Error()
			userPrompt = RenderRetryPrompt(originalUserMessage, []string{hintReturnJSONOnly})
			lastParsed = nil
			continue
		}

		result.Attempts++
		result.LastRawResponse = rawStdout

		var parsed AgentResponse
		if err := json.Unmarshal([]byte(stripCodeFence(agentText)), &parsed); err != nil {
			result.LastReason = fmt.Sprintf("json parse: %v", err)
			userPrompt = RenderRetryPrompt(originalUserMessage, []string{hintReturnJSONOnly})
			lastParsed = nil
			continue
		}
		lastParsed = &parsed

		validated, err := Validate(parsed, taxonomy)
		if err == nil {
			result.Outcome = OutcomeEnriched
			result.Summary = parsed.Summary
			result.LastReason = ""
			result.Classification = &validated
			return result
		}

		var vf ValidationFailure
		if errors.As(err, &vf) {
			result.LastReason = vf.Error()
			userPrompt = RenderRetryPrompt(originalUserMessage, vf.Hints)
			continue
		}

		// Validate only returns ValidationFailure or nil; reaching here means
		// a contract regression. Treat it as a parse miss and retry with the
		// generic "return JSON only" hint — never forward a raw Go error
		// string to the agent.
		result.LastReason = err.Error()
		userPrompt = RenderRetryPrompt(originalUserMessage, []string{hintReturnJSONOnly})
	}

	// Budget exhausted. Distinguish a parse miss (never got a valid
	// AgentResponse) from a validation miss (parsed but never passed rules).
	if lastParsed == nil {
		result.Outcome = OutcomeJSONFailed
	} else {
		result.Outcome = OutcomeValidationFailed
		result.Summary = lastParsed.Summary
	}
	return result
}

