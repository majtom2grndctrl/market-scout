package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Record is one sidecar JSONL row. See agent-context/lib/watchlist.md
// §Research file annotation for the canonical schema. The struct uses pointer
// strings for nullable annotation/tool fields so unset fields marshal as JSON
// null (per schema requirement: "Unset fields are null, not omitted").
//
// json.Unmarshal of a "null" value into a *string leaves the pointer nil,
// which round-trips cleanly back to "null" on encode — preserving the schema
// invariant on every rewrite, even for records this tool does not touch.
type Record struct {
	Rank          int     `json:"rank"`
	Name          string  `json:"name"`
	Source        Source  `json:"source"`
	URL           *string `json:"url"`
	CareersURL    *string `json:"careers_url"`
	ATS           *string `json:"ats"`
	BoardToken    *string `json:"board_token"`
	Notes         *string `json:"notes"`
	Status        *string `json:"status"`
	VerifiedAt    *string `json:"verified_at"`
	VerifiedRunID *string `json:"verified_run_id"`
}

// Source is the immutable sub-object captured by the annotator from the
// research row. Fields are pointer types for nullable values per the schema.
type Source struct {
	Industry          *string `json:"industry"`
	Location          *string `json:"location"`
	YearFounded       *int    `json:"year_founded"`
	Employees         *int    `json:"employees"`
	EmployeeChangePct *int    `json:"employee_change_pct"`
}

// IsTerminal reports whether the record carries a terminal status.
func (r *Record) IsTerminal() bool { return r.Status != nil }

// IsVerified reports whether the record has been stamped verified.
func (r *Record) IsVerified() bool { return r.VerifiedAt != nil }

// readSidecar parses a JSONL file. Blank lines are skipped. A parse error on
// any line aborts the read — the file is the operator's source of truth and
// silently dropping malformed records would lose information.
func readSidecar(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening sidecar %s: %w", path, err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	// Default Scanner buffer (64 KiB) is too small for a record with a long
	// notes field; raise to 1 MiB per line. A record exceeding 1 MiB is an
	// operator error; readSidecar returns an error and the run aborts rather
	// than silently dropping the record.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("sidecar %s line %d: %w", path, lineNum, err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning sidecar %s: %w", path, err)
	}
	return records, nil
}

// writeSidecar rewrites the sidecar atomically: temp file in the same
// directory, fsync, then rename. A crash mid-write leaves either the
// pre-write or post-write state on disk — never a half-written record.
//
// The temp file is created in the same directory as the target so the rename
// stays within one filesystem. On Linux and macOS, same-filesystem rename(2)
// is atomic at the OS level; cross-filesystem renames are not, which is why
// the temp file must live alongside the target.
func writeSidecar(path string, records []Record) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".onboard-*.jsonl.tmp")
	if err != nil {
		return fmt.Errorf("creating temp sidecar in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	// Best-effort cleanup if anything below this point fails. After the
	// rename succeeds, the temp path no longer exists and the Remove is a
	// harmless no-op.
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	w := bufio.NewWriter(tmp)
	enc := json.NewEncoder(w)
	// Pretty print would break "one JSON object per line" and confuse
	// downstream jq filters; default encoder writes compact form + trailing
	// newline, which is exactly the JSONL shape we want.
	for i, rec := range records {
		if err := enc.Encode(&rec); err != nil {
			cleanup()
			return fmt.Errorf("encoding record %d (rank=%d): %w", i, rec.Rank, err)
		}
	}
	if err := w.Flush(); err != nil {
		cleanup()
		return fmt.Errorf("flushing temp sidecar: %w", err)
	}
	// True crash-safety would also fsync the parent directory after rename;
	// we accept that gap.
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("fsyncing temp sidecar: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp sidecar: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp sidecar over %s: %w", path, err)
	}
	return nil
}

// strPtr returns a pointer to s. Helper for building literal records and
// stamping nullable fields without scattering temporaries through the code.
func strPtr(s string) *string { return &s }
