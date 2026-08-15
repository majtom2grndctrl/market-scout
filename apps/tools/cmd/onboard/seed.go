package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// seedRow is one row pending append to apps/tools/internal/db/seeds/companies.sql.
// Industry mirrors the seed file's `industry` column; an empty string emits
// SQL NULL, matching the seeded rows that lack an industry classification.
type seedRow struct {
	Name       string
	ATS        string
	BoardToken string
	Industry   string
	// Sectioning hint. The spec calls for grouping appended rows under an
	// ATS-section comment that names the source list and date (e.g.
	// "Greenhouse (GeekWire 200, May 2026)"). Group is the parenthetical;
	// the binary derives the ATS prefix from ATS.
	Group string
}

// seedAppender groups pending rows and appends them to the seed file in a
// single transactional write. Existing (ats, board_token) pairs already in
// the file are de-duped so a re-run is a no-op.
//
// Why dedup against the file: the DB's UNIQUE constraint catches duplicates
// at apply time, but appending a stale INSERT keeps growing the file's diff
// against git on every re-run. File-level dedup keeps the artifact stable.
type seedAppender struct {
	path     string
	existing map[string]struct{} // key: "ats|board_token"
}

func newSeedAppender(path string) (*seedAppender, error) {
	a := &seedAppender{path: path, existing: make(map[string]struct{})}
	if err := a.scanExisting(); err != nil {
		return nil, err
	}
	return a, nil
}

// seedRowKey returns the dedup key for an (ats, board_token) pair. Matches
// the DB's unique constraint and the spec's idempotence requirement.
func seedRowKey(ats, boardToken string) string {
	return ats + "|" + boardToken
}

// seedValuesRowRegex matches one VALUES row in the seed file:
//
//	('Name', 'ats', 'board_token', 'industry')
//
// The capture indexes used are ats=1, board_token=2.
//
// The name column uses (?:[^']|”)* to allow SQL-escaped apostrophes (” per
// ANSI SQL). The ats and board_token columns use [^']* — neither contains
// apostrophes and the simpler form is easier to read at those positions.
// Option (a) was chosen over a line-by-line tokenizer: the single pattern
// change is proportionate to the fix and the seed file's column order is
// stable.
//
// Assumes one VALUES row per line; multi-line rows would silently bypass
// in-file dedup. Future editors: keep rows single-line.
var seedValuesRowRegex = regexp.MustCompile(`\(\s*'(?:[^']|'')*'\s*,\s*'([^']*)'\s*,\s*'([^']*)'`)

func (a *seedAppender) scanExisting() error {
	f, err := os.Open(a.path)
	if err != nil {
		// Spec invariant: the seed file is the canonical source of truth
		// and is checked into the repo. If it is missing, surface the error
		// — auto-creating would mask a deeper repo-state problem.
		return fmt.Errorf("opening seed file %s: %w", a.path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip pure comment / blank lines fast — they cannot match the regex
		// but the early-out is documented because seed files use `-- ...`
		// section comments heavily.
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		matches := seedValuesRowRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		a.existing[seedRowKey(matches[1], matches[2])] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanning seed file %s: %w", a.path, err)
	}
	return nil
}

// Has reports whether the seed file already has a row for the given pair.
func (a *seedAppender) Has(ats, boardToken string) bool {
	_, ok := a.existing[seedRowKey(ats, boardToken)]
	return ok
}

// Append writes the queued rows to the seed file as one new self-contained
// INSERT statement per ATS group. No-op when rows is empty so a clean run
// (zero newly-verified companies) does not bump the file's mtime.
//
// Statement shape matches the existing file: multi-row INSERT terminated by
// ON CONFLICT (ats, board_token) DO NOTHING. Rows are not spliced into the
// existing INSERT — appended as a new statement, per the spec.
func (a *seedAppender) Append(rows []seedRow) error {
	if len(rows) == 0 {
		return nil
	}

	// Reject names containing control characters before touching the file.
	// A newline in a company name would produce a multi-line SQL row that
	// breaks the in-file dedup regex on subsequent runs. Silent normalization
	// would hide annotator errors, so we reject instead.
	for _, r := range rows {
		if strings.ContainsAny(r.Name, "\n\r\t\x00") {
			return fmt.Errorf("seed row for board_token %q: name contains control characters", r.BoardToken)
		}
	}

	// Group by (ats, Group). One INSERT per group keeps the diff structured
	// and matches the style of the file's existing batch-comment blocks —
	// each research-list + ATS combination gets its own INSERT statement.
	type groupKey struct {
		ATS   string
		Group string
	}
	grouped := make(map[groupKey][]seedRow)
	var order []groupKey
	for _, r := range rows {
		k := groupKey{ATS: r.ATS, Group: r.Group}
		if _, seen := grouped[k]; !seen {
			order = append(order, k)
		}
		grouped[k] = append(grouped[k], r)
	}

	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening seed file for append: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, k := range order {
		groupRows := grouped[k]
		// Leading blank line + section comment so the appended block reads
		// cleanly when diffed against the previous file state.
		if _, err := fmt.Fprintf(w, "\n-- %s (%s)\n", titleCaseATS(k.ATS), k.Group); err != nil {
			return fmt.Errorf("writing seed section header: %w", err)
		}
		if _, err := fmt.Fprintln(w, "INSERT INTO companies (name, ats, board_token, industry) VALUES"); err != nil {
			return fmt.Errorf("writing seed insert header: %w", err)
		}
		for i, row := range groupRows {
			sep := ","
			if i == len(groupRows)-1 {
				sep = ""
			}
			if _, err := fmt.Fprintf(w, "    (%s, %s, %s, %s)%s\n",
				sqlString(row.Name),
				sqlString(row.ATS),
				sqlString(row.BoardToken),
				sqlStringOrNull(row.Industry),
				sep,
			); err != nil {
				return fmt.Errorf("writing seed row: %w", err)
			}
		}
		if _, err := fmt.Fprintln(w, "ON CONFLICT (ats, board_token) DO NOTHING;"); err != nil {
			return fmt.Errorf("writing seed insert trailer: %w", err)
		}
		// Update in-memory dedup index so a second Append within the same
		// process (currently not used, but cheap insurance) is also idempotent.
		for _, row := range groupRows {
			a.existing[seedRowKey(row.ATS, row.BoardToken)] = struct{}{}
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing seed file: %w", err)
	}
	return nil
}

// sqlString quotes a literal for the seed file's single-quoted style.
// Escapes embedded single quotes by doubling them per ANSI SQL. No values
// in the current pipeline carry quotes, but the escape is here so a future
// company name with an apostrophe doesn't silently corrupt the file.
func sqlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// sqlStringOrNull renders the SQL NULL literal for empty strings; the seed
// file's industry column is nullable and the current rows all have a value,
// but an annotator can legitimately leave industry blank.
func sqlStringOrNull(s string) string {
	if s == "" {
		return "NULL"
	}
	return sqlString(s)
}

// titleCaseATS maps the lowercase ats enum to the display form used in the
// existing seed file's section comments (e.g. "greenhouse" -> "Greenhouse").
func titleCaseATS(ats string) string {
	switch ats {
	case "greenhouse":
		return "Greenhouse"
	case "lever":
		return "Lever"
	case "ashby":
		return "Ashby"
	case "workday":
		return "Workday"
	case "workable":
		return "Workable"
	case "gem":
		return "Gem"
	default:
		// Defensive: caller has already filtered to supported ATS values.
		// Capitalize defensively rather than panicking.
		if ats == "" {
			return ""
		}
		return strings.ToUpper(ats[:1]) + ats[1:]
	}
}
