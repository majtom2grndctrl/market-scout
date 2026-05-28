package ats

import (
	"os"
	"path/filepath"
	"testing"
)

// loadAdapterFixture reads a recorded ATS JSON response from
// testdata/<adapter>/<name> and returns its raw bytes. Fixtures are
// recorded real API responses; name is the filename (e.g. "jobs_full.json").
// The test fails immediately if the file cannot be read.
func loadAdapterFixture(t *testing.T, adapter, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", adapter, name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
}
