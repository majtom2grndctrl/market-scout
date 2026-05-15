package main

import (
	"flag"
	"math"
	"regexp"
	"strings"
	"testing"
)

// identifierPattern is the safe character set that pinned constants must
// match so they can be embedded in filenames, log lines, and DB rows without
// escaping. Tested here rather than enforced at runtime because the values
// are compile-time constants — a violation is a code edit, not a user error.
var testIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func TestConstants_MatchIdentifierPattern(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{"PromptVersion", PromptVersion},
		{"Model", Model},
	} {
		if !testIdentifierPattern.MatchString(tc.value) {
			t.Errorf("constant %s = %q does not match identifier pattern %s",
				tc.name, tc.value, testIdentifierPattern)
		}
	}
}

// validConfig returns a Config that passes Validate. Tests mutate copies to
// isolate individual checks.
func validConfig() Config {
	return Config{
		PromptVersion:     PromptVersion,
		Model:             Model,
		Count:             10,
		WaveSize:          10,
		BatchSize:         5,
		MaxRetries:        3,
		MaxParallelAgents: 10,
		ReportFormat:      "json",
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestConfig_Validate_CountZero(t *testing.T) {
	cfg := validConfig()
	cfg.Count = 0
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--count") {
		t.Fatalf("expected --count error, got: %v", err)
	}
}

func TestConfig_Validate_CountNegative(t *testing.T) {
	cfg := validConfig()
	cfg.Count = -1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--count") {
		t.Fatalf("expected --count error, got: %v", err)
	}
}

func TestConfig_Validate_CountExceedsMaxInt32(t *testing.T) {
	cfg := validConfig()
	cfg.Count = math.MaxInt32 + 1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--count") {
		t.Fatalf("expected --count error, got: %v", err)
	}
}

func TestConfig_Validate_WaveSizeZero(t *testing.T) {
	cfg := validConfig()
	cfg.WaveSize = 0
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--wave-size") {
		t.Fatalf("expected --wave-size error, got: %v", err)
	}
}

func TestConfig_Validate_BatchSizeZero(t *testing.T) {
	cfg := validConfig()
	cfg.BatchSize = 0
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--batch-size") {
		t.Fatalf("expected --batch-size error, got: %v", err)
	}
}

func TestConfig_Validate_BatchSizeNegative(t *testing.T) {
	cfg := validConfig()
	cfg.BatchSize = -1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--batch-size") {
		t.Fatalf("expected --batch-size error, got: %v", err)
	}
}

func TestConfig_Validate_BatchSizeOne_Valid(t *testing.T) {
	cfg := validConfig()
	cfg.BatchSize = 1
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for BatchSize=1, got: %v", err)
	}
}

func TestConfig_Validate_MaxRetriesNegative(t *testing.T) {
	cfg := validConfig()
	cfg.MaxRetries = -1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--max-retries") {
		t.Fatalf("expected --max-retries error, got: %v", err)
	}
}

func TestConfig_Validate_MaxRetriesZero_Valid(t *testing.T) {
	cfg := validConfig()
	cfg.MaxRetries = 0 // zero is allowed — means no retries
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for MaxRetries=0, got: %v", err)
	}
}

func TestConfig_Validate_MaxParallelAgentsZero(t *testing.T) {
	cfg := validConfig()
	cfg.MaxParallelAgents = 0
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--max-parallel") {
		t.Fatalf("expected --max-parallel error, got: %v", err)
	}
}

func TestConfig_Validate_InvalidReportFormat(t *testing.T) {
	cfg := validConfig()
	cfg.ReportFormat = "csv"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "--report-format") {
		t.Fatalf("expected --report-format error, got: %v", err)
	}
}

func TestParseFlags_RejectsNegativeMaxRetries(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	_, err := ParseFlags(fs, []string{"-max-retries=-1"})
	if err == nil || !strings.Contains(err.Error(), "--max-retries") {
		t.Fatalf("expected --max-retries validation error, got: %v", err)
	}
}
