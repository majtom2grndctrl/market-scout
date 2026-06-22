package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

type fakeQueryRows struct {
	cols   []string
	values [][]any
	pos    int
}

func (f *fakeQueryRows) Columns() ([]string, error) {
	return f.cols, nil
}

func (f *fakeQueryRows) Next() bool {
	if f.pos >= len(f.values) {
		return false
	}
	f.pos++
	return true
}

func (f *fakeQueryRows) Scan(dest ...any) error {
	row := f.values[f.pos-1]
	for i := range dest {
		value := dest[i].(*any)
		*value = row[i]
	}
	return nil
}

func (f *fakeQueryRows) Err() error {
	return nil
}

func TestScanQueryRows_EncodesTypesNullsAndTruncation(t *testing.T) {
	observedAt := time.Date(2026, 6, 17, 19, 30, 45, 0, time.FixedZone("PDT", -7*60*60))
	rows := &fakeQueryRows{
		cols: []string{"id", "name", "observed_at", "missing"},
		values: [][]any{
			{int64(42), "Acme", observedAt, nil},
			{int64(43), "Overflow", observedAt, nil},
		},
	}

	got, err := scanQueryRows(rows, 1)
	if err != nil {
		t.Fatalf("scanQueryRows: %v", err)
	}

	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := `{"rows":[{"id":42,"missing":null,"name":"Acme","observed_at":"2026-06-18T02:30:45Z"}],"row_count":1,"truncated":true}`
	if string(payload) != want {
		t.Fatalf("payload:\n got: %s\nwant: %s", payload, want)
	}
}

func TestScanQueryRows_NormalizesByteJSONAndText(t *testing.T) {
	rows := &fakeQueryRows{
		cols: []string{"payload", "note"},
		values: [][]any{
			{
				[]byte(`{"role":"backend","skills":["go"]}`),
				[]byte("plain text"),
			},
		},
	}

	got, err := scanQueryRows(rows, 10)
	if err != nil {
		t.Fatalf("scanQueryRows: %v", err)
	}

	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := `{"rows":[{"note":"plain text","payload":{"role":"backend","skills":["go"]}}],"row_count":1,"truncated":false}`
	if string(payload) != want {
		t.Fatalf("payload:\n got: %s\nwant: %s", payload, want)
	}
}

func TestRun_ExitsNonZeroWhenDatabaseURLROUnset(t *testing.T) {
	t.Setenv("DATABASE_URL_RO", "")

	var stderr bytes.Buffer
	got := run(nil, io.Discard, &stderr)
	if got == exitOK {
		t.Fatalf("run exit code = %d, want non-zero", got)
	}
	if !strings.Contains(stderr.String(), "DATABASE_URL_RO is not set") {
		t.Fatalf("stderr = %q, want DATABASE_URL_RO unset message", stderr.String())
	}
}

func TestMapFetchStatusRow_MarshalsInvalidNullsAsJSONNull(t *testing.T) {
	startedAt := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	row := mapFetchStatusRow(db.ListLatestFetchRunsByCompanyRow{
		Name:          "Acme",
		Status:        "in_progress",
		StartedAt:     startedAt,
		CompletedAt:   sql.NullTime{},
		PostingsCount: sql.NullInt32{},
		ErrorMessage:  sql.NullString{},
	})

	payload, err := json.Marshal(fetchStatusEnvelope{
		Rows:      []fetchStatusRow{row},
		RowCount:  1,
		Truncated: false,
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := `{"rows":[{"company":"Acme","status":"in_progress","started_at":"2026-06-17T12:00:00Z","completed_at":null,"postings_count":null,"error_message":null}],"row_count":1,"truncated":false}`
	if string(payload) != want {
		t.Fatalf("payload:\n got: %s\nwant: %s", payload, want)
	}
}
