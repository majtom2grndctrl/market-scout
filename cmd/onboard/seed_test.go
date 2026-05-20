package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalSeedFile is the smallest valid seed file the appender requires.
// Mirrors the production shape: one INSERT with an ON CONFLICT trailer.
const minimalSeedFile = `-- header comment
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Existing Co', 'greenhouse', 'existing', 'AI')
ON CONFLICT (ats, board_token) DO NOTHING;
`

// writeSeedFixture writes content to a temp-dir seed file and returns its path.
func writeSeedFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "companies.sql")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write seed fixture: %v", err)
	}
	return path
}

// TestSeedAppender_ApostropheDedup verifies that a company name containing a
// SQL-escaped apostrophe (written as '' by sqlString) is recognised by the
// in-file dedup regex on a subsequent run, so Has returns true and no duplicate
// row is appended.
//
// The dedup contract: callers check Has before adding a row to the pending
// slice. If the regex missed the apostrophe row, Has would return false and a
// second run would append a duplicate. This test drives that exact flow.
func TestSeedAppender_ApostropheDedup(t *testing.T) {
	path := writeSeedFixture(t, minimalSeedFile)

	row := seedRow{
		Name:       "L'Oreal",
		ATS:        "greenhouse",
		BoardToken: "loreal",
		Industry:   "consumer goods",
		Group:      "Test, May 2026",
	}

	// First append: must succeed and write the row with the SQL-escaped apostrophe.
	a1, err := newSeedAppender(path)
	if err != nil {
		t.Fatalf("newSeedAppender: %v", err)
	}
	if err := a1.Append([]seedRow{row}); err != nil {
		t.Fatalf("first Append: %v", err)
	}

	// Verify the file now contains the SQL-escaped form.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed file: %v", err)
	}
	if !strings.Contains(string(contents), "'L''Oreal'") {
		t.Errorf("seed file should contain SQL-escaped apostrophe; got:\n%s", contents)
	}

	// Second run: a fresh seedAppender must recognise the row as already present
	// via Has. If the regex blind spot were still present, Has would return false
	// and the caller's dedup guard would be bypassed, leading to a duplicate.
	a2, err := newSeedAppender(path)
	if err != nil {
		t.Fatalf("newSeedAppender (second run): %v", err)
	}
	if !a2.Has("greenhouse", "loreal") {
		t.Error("Has: apostrophe row not recognised after reload — dedup regex missed it; second run would produce a duplicate")
	}

	// Simulate the caller's dedup guard: only append rows where Has is false.
	// Because Has is now true, pendingSeed is empty and the seed file is unchanged.
	var pendingSeed []seedRow
	if !a2.Has(row.ATS, row.BoardToken) {
		pendingSeed = append(pendingSeed, row)
	}
	contentsBefore, _ := os.ReadFile(path)
	if err := a2.Append(pendingSeed); err != nil {
		t.Fatalf("second Append: %v", err)
	}
	contentsAfter, _ := os.ReadFile(path)
	if string(contentsBefore) != string(contentsAfter) {
		t.Errorf("second run changed seed file — expected no-op:\nbefore:\n%s\nafter:\n%s",
			contentsBefore, contentsAfter)
	}
}

// TestSeedAppender_ControlCharInNameReturnsError verifies that Append rejects
// a company name containing embedded newlines (or other control characters)
// rather than silently writing a malformed SQL row.
func TestSeedAppender_ControlCharInNameReturnsError(t *testing.T) {
	cases := []struct {
		desc string
		name string
	}{
		{"newline", "Bad\nName"},
		{"carriage return", "Bad\rName"},
		{"tab", "Bad\tName"},
		{"null byte", "Bad\x00Name"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			path := writeSeedFixture(t, minimalSeedFile)
			a, err := newSeedAppender(path)
			if err != nil {
				t.Fatalf("newSeedAppender: %v", err)
			}
			row := seedRow{
				Name:       tc.name,
				ATS:        "greenhouse",
				BoardToken: "badco",
				Group:      "Test, May 2026",
			}
			if err := a.Append([]seedRow{row}); err == nil {
				t.Errorf("Append: expected error for name with %s, got nil", tc.desc)
			}
			// The seed file must be unchanged — the error must fire before any write.
			contents, _ := os.ReadFile(path)
			if string(contents) != minimalSeedFile {
				t.Errorf("seed file was modified despite error:\n%s", contents)
			}
		})
	}
}
