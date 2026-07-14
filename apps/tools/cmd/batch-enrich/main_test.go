package main

import (
	"errors"
	"strings"
	"testing"
)

func TestNewAgentRunner_PreflightsOnlySelectedExecutable(t *testing.T) {
	originalLookPath := runnerLookPath
	t.Cleanup(func() { runnerLookPath = originalLookPath })

	for _, tc := range []struct {
		name       string
		runner     string
		model      string
		executable string
	}{
		{
			name:       "codex exec",
			runner:     RunnerCodexExec,
			model:      CodexExecModel,
			executable: "codex",
		},
		{
			name:       "claude",
			runner:     RunnerClaude,
			model:      ClaudeModel,
			executable: "claude",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var lookedUp []string
			runnerLookPath = func(name string) (string, error) {
				lookedUp = append(lookedUp, name)
				return "/test/bin/" + name, nil
			}

			runner, cleanup, err := newAgentRunner(Config{
				Runner: tc.runner,
				Model:  tc.model,
			})
			if err != nil {
				t.Fatalf("newAgentRunner: %v", err)
			}
			if runner == nil || cleanup == nil {
				t.Fatalf("newAgentRunner returned nil runner or cleanup: runner_nil=%t cleanup_nil=%t", runner == nil, cleanup == nil)
			}
			if len(lookedUp) != 1 || lookedUp[0] != tc.executable {
				t.Fatalf("preflighted %v, want only %q", lookedUp, tc.executable)
			}
			if err := cleanup(); err != nil {
				t.Fatalf("cleanup: %v", err)
			}
		})
	}
}

func TestNewAgentRunner_PreflightFailureStopsConstruction(t *testing.T) {
	originalLookPath := runnerLookPath
	t.Cleanup(func() { runnerLookPath = originalLookPath })

	runnerLookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}

	runner, cleanup, err := newAgentRunner(Config{
		Runner: RunnerCodexExec,
		Model:  CodexExecModel,
	})
	if err == nil {
		t.Fatal("newAgentRunner succeeded despite missing selected executable")
	}
	if runner != nil || cleanup != nil {
		t.Fatalf("newAgentRunner returned runner or cleanup after preflight failure: runner_nil=%t cleanup_nil=%t", runner == nil, cleanup == nil)
	}
	if !strings.Contains(err.Error(), "codex CLI not found in PATH") {
		t.Fatalf("error = %v, want actionable codex CLI error", err)
	}
}
