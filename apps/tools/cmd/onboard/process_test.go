package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/atsdetect"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/domain"
)

// fakeAdapter returns a controllable adapter used in every status-path test.
// FetchPostings returns wantErr verbatim, so a test can assert on a probe's
// success-or-failure branch without standing up a full httptest server for
// every ATS variant. The httptest server in TestRun_VerifiedAndInvalidToken
// exercises the real ats.Greenhouse code path end to end.
type fakeAdapter struct {
	wantErr error
}

func (f *fakeAdapter) FetchPostings(_ context.Context, _ string) ([]domain.Posting, error) {
	if f.wantErr != nil {
		return nil, f.wantErr
	}
	return []domain.Posting{}, nil
}

// fakeQuerier captures the (ats, board_token) pair the processor asks about
// and returns a scripted result. err==sql.ErrNoRows triggers the "no match"
// branch in process.go.
type fakeQuerier struct {
	row db.FindCompanyDedupStatusRow
	err error
}

func (f *fakeQuerier) FindCompanyDedupStatus(_ context.Context, _ db.FindCompanyDedupStatusParams) (db.FindCompanyDedupStatusRow, error) {
	return f.row, f.err
}

// careersServer returns an httptest server that responds 200 to every
// request. Callers point Record.CareersURL at srv.URL and the careers probe
// succeeds. For the "no-careers" path the test uses a server that returns 404.
func careersServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withAdapter swaps a package-level adapter constructor for the test's
// duration. Each test that needs a controllable adapter calls this helper
// and the Cleanup hook restores the production constructor at end-of-test.
// Tests run sequentially in the default config, so the swap is safe; if a
// future migration to t.Parallel breaks this, the adapter constructors
// should move onto the processor struct instead.
func withAdapter(t *testing.T, atsName string, want *fakeAdapter) {
	t.Helper()
	switch atsName {
	case "greenhouse":
		orig := atsNewGreenhouse
		atsNewGreenhouse = func(*http.Client) adapterProbe { return want }
		t.Cleanup(func() { atsNewGreenhouse = orig })
	case "lever":
		orig := atsNewLever
		atsNewLever = func(*http.Client) adapterProbe { return want }
		t.Cleanup(func() { atsNewLever = orig })
	case "ashby":
		orig := atsNewAshby
		atsNewAshby = func(*http.Client) adapterProbe { return want }
		t.Cleanup(func() { atsNewAshby = orig })
	case "workday":
		orig := atsNewWorkday
		atsNewWorkday = func(*http.Client) adapterProbe { return want }
		t.Cleanup(func() { atsNewWorkday = orig })
	case "workable":
		orig := atsNewWorkable
		atsNewWorkable = func(*http.Client) adapterProbe { return want }
		t.Cleanup(func() { atsNewWorkable = orig })
	default:
		t.Fatalf("withAdapter: unknown ats %q", atsName)
	}
}

func newTestProcessor(t *testing.T, q dedupQuerier) *processor {
	t.Helper()
	return &processor{
		queries:    q,
		httpClient: &http.Client{},
		runID:      "01TESTRUN0000000000000000",
		now:        func() time.Time { return time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC) },
		seedGroup:  "Test, May 2026",
	}
}

func TestProcessRecord_VerifiedPath(t *testing.T) {
	withAdapter(t, "greenhouse", &fakeAdapter{})

	careers := careersServer(t, http.StatusOK)
	rec := Record{
		Rank: 1, Name: "Acme",
		URL:        strPtr("https://acme.example.com"),
		CareersURL: strPtr(careers.URL),
		ATS:        strPtr("greenhouse"),
		BoardToken: strPtr("acme"),
		// Source.Industry set to prove it does NOT propagate into the seed
		// row — source taxonomies vary per research source and must not
		// leak into the canonical companies.industry column.
		Source: Source{Industry: strPtr("AI")},
	}
	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}

	if !res.verified {
		t.Fatalf("verified: got false, want true; status=%q", res.status)
	}
	// VerifiedAt must equal the injected now() formatted as RFC3339 UTC.
	wantVerifiedAt := "2026-05-19T12:00:00Z"
	if rec.VerifiedAt == nil || *rec.VerifiedAt != wantVerifiedAt {
		t.Errorf("VerifiedAt: got %v, want %q", rec.VerifiedAt, wantVerifiedAt)
	}
	if rec.VerifiedRunID == nil || *rec.VerifiedRunID != "01TESTRUN0000000000000000" {
		t.Errorf("VerifiedRunID: got %v, want 01TESTRUN0000000000000000", rec.VerifiedRunID)
	}
	if rec.Status != nil {
		t.Errorf("Status: got %v, want nil for verified record", rec.Status)
	}
	if res.seedRow == nil {
		t.Fatal("seedRow: got nil, want populated for verified row")
	}
	if res.seedRow.ATS != "greenhouse" || res.seedRow.BoardToken != "acme" {
		t.Errorf("seedRow: got (%s,%s), want (greenhouse,acme)", res.seedRow.ATS, res.seedRow.BoardToken)
	}
	if res.seedRow.Industry != "" {
		t.Errorf("seedRow.Industry: got %q, want empty — source industry must not propagate", res.seedRow.Industry)
	}
}

func TestProcessRecord_DuplicateFromDB(t *testing.T) {
	// Adapter constructor is not swapped: the dedup short-circuit must fire
	// before any ATS probe runs. If the processor incorrectly reaches the
	// real adapter constructor it would still build a Greenhouse client; the
	// test asserts on rec.Status, which the dedup branch sets directly.
	rec := Record{
		Rank: 2, Name: "DupCo",
		URL:        strPtr("https://dupco.example.com"),
		CareersURL: strPtr("https://boards.greenhouse.io/dupco"),
		ATS:        strPtr("greenhouse"),
		BoardToken: strPtr("dupco"),
	}
	q := &fakeQuerier{row: db.FindCompanyDedupStatusRow{CompanyID: 7, HasRecentSnapshot: true}, err: nil}
	p := newTestProcessor(t, q)
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}

	if res.status != "duplicate" {
		t.Fatalf("status: got %q, want duplicate", res.status)
	}
	if rec.Status == nil || *rec.Status != "duplicate" {
		t.Errorf("rec.Status: got %v, want duplicate", rec.Status)
	}
	if rec.VerifiedAt != nil {
		t.Errorf("VerifiedAt: got %v, want nil for terminal status", rec.VerifiedAt)
	}
}

func TestProcessRecord_StaleNeedsMerge(t *testing.T) {
	rec := Record{
		Rank: 3, Name: "StaleCo",
		URL:        strPtr("https://staleco.example.com"),
		CareersURL: strPtr("https://boards.greenhouse.io/staleco"),
		ATS:        strPtr("greenhouse"),
		BoardToken: strPtr("staleco"),
	}
	q := &fakeQuerier{row: db.FindCompanyDedupStatusRow{CompanyID: 9, HasRecentSnapshot: false}, err: nil}
	p := newTestProcessor(t, q)
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}

	if res.status != "stale-needs-merge" {
		t.Fatalf("status: got %q, want stale-needs-merge", res.status)
	}
	if rec.Status == nil || *rec.Status != "stale-needs-merge" {
		t.Errorf("rec.Status: got %v, want stale-needs-merge", rec.Status)
	}
}

func TestProcessRecord_InvalidToken(t *testing.T) {
	withAdapter(t, "greenhouse", &fakeAdapter{wantErr: errProbeFailed})

	careers := careersServer(t, http.StatusOK)
	rec := Record{
		Rank: 4, Name: "BadToken",
		URL:        strPtr("https://badtoken.example.com"),
		CareersURL: strPtr(careers.URL),
		ATS:        strPtr("greenhouse"),
		BoardToken: strPtr("nope"),
	}
	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}

	if res.status != "invalid-token" {
		t.Fatalf("status: got %q, want invalid-token", res.status)
	}
}

func TestProcessRecord_UnsupportedATS(t *testing.T) {
	// Tests the processRecord path when the careers URL probe succeeds but
	// the URL matches no detection pattern. We need an HTTP server (so the
	// probe gets a 2xx) AND assurance that the URL is unrecognized. Use a
	// server URL whose host is 127.0.0.1 — confirmed unrecognized by the
	// adjacent TestDetectURL_Rejection — and short-circuit if a future
	// regex set ever makes the assertion accidentally pass.
	careers := careersServer(t, http.StatusOK)
	if atsdetect.DetectURL(careers.URL, "careers_url").Recognized {
		t.Fatalf("test invariant broken: atsdetect.DetectURL(%q) recognized; pick a different URL", careers.URL)
	}
	rec := Record{
		Rank: 5, Name: "Random",
		URL:        strPtr("https://random.example.com"),
		CareersURL: strPtr(careers.URL),
	}

	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}
	if res.status != "unsupported-ats" {
		t.Fatalf("status: got %q, want unsupported-ats", res.status)
	}
}

// TestDetectURL_Rejection is a pure-unit assertion that ATS detection rejects
// URLs that don't match any known pattern. Kept separate from the
// processRecord path test so a future change to the detection rules that
// accidentally widens coverage surfaces here, not as a state-machine bug.
func TestDetectURL_Rejection(t *testing.T) {
	rejects := []string{
		"https://invalid.example.invalid/careers",
		"https://careers.random.example.com/jobs",
		"https://example.com/careers",
	}
	for _, u := range rejects {
		if got := atsdetect.DetectURL(u, "careers_url"); got.Recognized {
			t.Errorf("atsdetect.DetectURL(%q): got Recognized=true, want false", u)
		}
	}
}

func TestProcessRecord_NoCareers(t *testing.T) {
	// careers_url returns 404 and ats is nil → no-careers status.
	careers := careersServer(t, http.StatusNotFound)
	rec := Record{
		Rank: 6, Name: "NoCareers",
		URL:        strPtr("https://nocareers.example.com"),
		CareersURL: strPtr(careers.URL),
	}
	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}
	if res.status != "no-careers" {
		t.Fatalf("status: got %q, want no-careers", res.status)
	}
}

func TestProcessRecord_PreconditionUrlMissing(t *testing.T) {
	rec := Record{Rank: 7, Name: "NoURL"}
	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}
	if res.preconditionMissing != "url" {
		t.Fatalf("preconditionMissing: got %q, want url", res.preconditionMissing)
	}
	if rec.Status != nil {
		t.Errorf("Status: got %v, want nil — preconditions must not mutate record", rec.Status)
	}
}

// TestProcessRecord_PreconditionCareersMissing asserts a record with url
// set but neither careers_url nor ats reports careers_url as the missing
// precondition. Earlier behavior incorrectly stamped no-careers here; the
// no-careers status is reserved for actual probe failures so the annotator
// can decide whether the company has a careers page at all.
func TestProcessRecord_PreconditionCareersMissing(t *testing.T) {
	rec := Record{Rank: 70, Name: "NoCareersField", URL: strPtr("https://noc.example.com")}
	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}
	if res.preconditionMissing != "careers_url" {
		t.Fatalf("preconditionMissing: got %q, want careers_url", res.preconditionMissing)
	}
	if rec.Status != nil {
		t.Errorf("Status: got %v, want nil — precondition gaps must not mutate record", rec.Status)
	}
}

func TestProcessRecord_SkipsTerminal(t *testing.T) {
	rec := Record{Rank: 8, Name: "Done", Status: strPtr("dead")}
	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}
	if res.status != "dead" {
		t.Fatalf("status: got %q, want dead", res.status)
	}
}

func TestProcessRecord_SkipsAlreadyVerified(t *testing.T) {
	rec := Record{
		Rank: 9, Name: "Verified",
		URL:        strPtr("https://verified.example.com"),
		VerifiedAt: strPtr("2026-05-01T00:00:00Z"),
	}
	p := newTestProcessor(t, &fakeQuerier{err: sql.ErrNoRows})
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}
	if !res.verified {
		t.Fatalf("verified: got false, want true")
	}
	if res.seedRow != nil {
		t.Errorf("seedRow: got non-nil, want nil — already-verified records do not re-emit seed rows")
	}
}

// TestProcessRecord_DBErrorPropagates asserts that a non-sql.ErrNoRows DB
// failure surfaces as a real error from processRecord, not as a
// per-record precondition gap. The runner translates this into exit 1, not
// exit 2.
func TestProcessRecord_DBErrorPropagates(t *testing.T) {
	rec := Record{
		Rank: 71, Name: "DBDown",
		URL:        strPtr("https://dbdown.example.com"),
		CareersURL: strPtr("https://boards.greenhouse.io/dbdown"),
		ATS:        strPtr("greenhouse"),
		BoardToken: strPtr("dbdown"),
	}
	dbErr := errors.New("connection refused")
	p := newTestProcessor(t, &fakeQuerier{err: dbErr})
	res, err := p.processRecord(t.Context(), &rec)
	if err == nil {
		t.Fatalf("processRecord: got nil error, want non-nil from DB failure (outcome=%+v)", res)
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("processRecord error chain: missing wrapped DB error; got %v", err)
	}
	if res.preconditionMissing != "" {
		t.Errorf("preconditionMissing: got %q, want empty — DB failures are not precondition gaps", res.preconditionMissing)
	}
}

// TestProcessRecord_DedupAfterAutoDetect asserts the state machine re-runs
// the dedup check after careers-URL-based ATS detection populates ats and
// board_token. Without this, a record where the annotator left ats nil and
// supplied a recognizable careers_url would silently bypass dedup.
func TestProcessRecord_DedupAfterAutoDetect(t *testing.T) {
	rec := Record{
		Rank: 72, Name: "AutoDetectedDup",
		URL:        strPtr("https://autodup.example.com"),
		CareersURL: strPtr("https://boards.greenhouse.io/autodup"),
	}
	q := &fakeQuerier{row: db.FindCompanyDedupStatusRow{CompanyID: 11, HasRecentSnapshot: true}, err: nil}
	p := newTestProcessor(t, q)
	res, err := p.processRecord(t.Context(), &rec)
	if err != nil {
		t.Fatalf("processRecord error: %v", err)
	}
	if res.status != "duplicate" {
		t.Fatalf("status: got %q, want duplicate (dedup must run post-detection)", res.status)
	}
	if rec.ATS == nil || *rec.ATS != "greenhouse" {
		t.Errorf("rec.ATS: got %v, want greenhouse — detection should have fired before dedup", rec.ATS)
	}
}

func TestDetectURL(t *testing.T) {
	tests := []struct {
		url   string
		want  bool
		ats   string
		token string
	}{
		{"https://boards.greenhouse.io/acme", true, "greenhouse", "acme"},
		{"https://job-boards.greenhouse.io/acme/jobs", true, "greenhouse", "acme"},
		{"https://jobs.lever.co/acme", true, "lever", "acme"},
		{"https://jobs.ashbyhq.com/acme", true, "ashby", "acme"},
		{"https://acme.wd5.myworkdayjobs.com/en-US/AcmeCareers/jobs", true, "workday", "acme.wd5.myworkdayjobs.com/AcmeCareers"},
		{"https://acme.wd5.myworkdayjobs.com/AcmeCareers", true, "workday", "acme.wd5.myworkdayjobs.com/AcmeCareers"},
		{"https://apply.workable.com/AcmeRobotics/", true, "workable", "acmerobotics"},
		{"https://careers.random.example.com/jobs", false, "", ""},
	}
	for _, tc := range tests {
		got := atsdetect.DetectURL(tc.url, "careers_url")
		if got.Recognized != tc.want {
			t.Errorf("atsdetect.DetectURL(%q).Recognized: got %v, want %v", tc.url, got.Recognized, tc.want)
			continue
		}
		if tc.want {
			if got.ATS != tc.ats {
				t.Errorf("atsdetect.DetectURL(%q).ATS: got %q, want %q", tc.url, got.ATS, tc.ats)
			}
			if got.BoardToken != tc.token {
				t.Errorf("atsdetect.DetectURL(%q).BoardToken: got %q, want %q", tc.url, got.BoardToken, tc.token)
			}
		}
	}
}

// errProbeFailed is a sentinel error used by fake adapters to drive the
// invalid-token branch. It carries no behavior beyond identity.
var errProbeFailed = stringErr("probe failed")

type stringErr string

func (s stringErr) Error() string { return string(s) }

// TestRun_FullCycle exercises run() end-to-end against a fixture sidecar and
// a fixture seed file. Asserts: every status path lands in the expected
// place, JSONL writeback uses temp+rename (no .tmp files remain), seed file
// gains the expected INSERT for verified rows only, and a second run is a
// no-op (no duplicate seed rows, no field churn on already-final records).
func TestRun_FullCycle(t *testing.T) {
	// All adapters are stubbed so the test never touches the network.
	withAdapter(t, "greenhouse", &fakeAdapter{}) // success
	withAdapter(t, "lever", &fakeAdapter{wantErr: errProbeFailed})

	careersOK := careersServer(t, http.StatusOK)
	careers404 := careersServer(t, http.StatusNotFound)

	dir := t.TempDir()
	sidecarPath := filepath.Join(dir, "candidates.jsonl")
	seedPath := filepath.Join(dir, "companies.sql")

	// Seed file mirrors the production shape: one multi-row INSERT with an
	// ON CONFLICT trailer. The appender must not splice into this statement.
	if err := os.WriteFile(seedPath, []byte(`-- header comment
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Existing', 'greenhouse', 'existing', 'AI')
ON CONFLICT (ats, board_token) DO NOTHING;
`), 0o644); err != nil {
		t.Fatalf("seed setup: %v", err)
	}

	// Records in source-file order covering every status path.
	records := []Record{
		{
			Rank: 1, Name: "Verified Co",
			URL: strPtr("https://verifiedco.example.com"), CareersURL: strPtr(careersOK.URL),
			ATS: strPtr("greenhouse"), BoardToken: strPtr("verifiedco"),
			// Source.Industry set to prove the emitted seed row does NOT
			// inherit it — see assertion below on the appended row.
			Source: Source{Industry: strPtr("AI")},
		},
		{
			Rank: 2, Name: "Invalid Token Co",
			URL: strPtr("https://invalid.example.com"), CareersURL: strPtr(careersOK.URL),
			ATS: strPtr("lever"), BoardToken: strPtr("badslug"),
		},
		{
			Rank: 3, Name: "No Careers Co",
			URL: strPtr("https://nocareers.example.com"), CareersURL: strPtr(careers404.URL),
		},
		{
			Rank: 4, Name: "Unsupported Co",
			URL: strPtr("https://unsupported.example.com"), CareersURL: strPtr(careersOK.URL),
		},
		{
			Rank: 5, Name: "Already Dead",
			URL: strPtr("https://dead.example.com"), Status: strPtr("dead"),
		},
	}
	writeFixtureSidecar(t, sidecarPath, records)

	// First run: no DB (every record looks fresh).
	stdoutPath := filepath.Join(dir, "stdout.json")
	code := runWithCapturedStdout(t, []string{"-no-db", "-seed", seedPath, "-group", "Test, May 2026", sidecarPath}, stdoutPath)
	if code != exitOK {
		t.Fatalf("first run exit: got %d, want %d", code, exitOK)
	}

	// Sidecar must reflect expected per-record outcomes.
	after := readFixtureSidecar(t, sidecarPath)
	if len(after) != len(records) {
		t.Fatalf("sidecar length: got %d, want %d", len(after), len(records))
	}
	if after[0].VerifiedAt == nil || after[0].Status != nil {
		t.Errorf("rank 1: want verified, got verified_at=%v status=%v", after[0].VerifiedAt, after[0].Status)
	}
	if after[1].Status == nil || *after[1].Status != "invalid-token" {
		t.Errorf("rank 2: want status=invalid-token, got %v", after[1].Status)
	}
	if after[2].Status == nil || *after[2].Status != "no-careers" {
		t.Errorf("rank 3: want status=no-careers, got %v", after[2].Status)
	}
	if after[3].Status == nil || *after[3].Status != "unsupported-ats" {
		t.Errorf("rank 4: want status=unsupported-ats, got %v", after[3].Status)
	}
	if after[4].Status == nil || *after[4].Status != "dead" {
		t.Errorf("rank 5: pre-set status mutated to %v", after[4].Status)
	}

	// No temp files should linger after a clean run — writeback uses rename
	// and the appender Append uses os.OpenFile in-place.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file lingered: %s", e.Name())
		}
	}

	// Seed file must contain exactly one new INSERT block with verifiedco.
	seedContents := readFile(t, seedPath)
	if !strings.Contains(seedContents, "'verifiedco'") {
		t.Errorf("seed file missing verifiedco row:\n%s", seedContents)
	}
	if !strings.Contains(seedContents, "Test, May 2026") {
		t.Errorf("seed file missing section comment for group")
	}
	// The record's Source.Industry ("AI") must not have leaked into the
	// emitted row: onboard always emits an empty industry (NULL), assigned
	// deliberately later rather than inherited from the source.
	if !strings.Contains(seedContents, "'verifiedco', NULL)") {
		t.Errorf("seed file: verifiedco row industry got non-NULL, want NULL:\n%s", seedContents)
	}

	// Summary on stdout must be valid JSON and reflect the counts.
	var sum summary
	if err := json.Unmarshal(readBytes(t, stdoutPath), &sum); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if sum.Verified != 1 {
		t.Errorf("summary.verified: got %d, want 1", sum.Verified)
	}
	if sum.ByStatus["invalid-token"] != 1 || sum.ByStatus["no-careers"] != 1 || sum.ByStatus["unsupported-ats"] != 1 {
		t.Errorf("summary.by_status: %+v", sum.ByStatus)
	}

	// Idempotence: a second run is a no-op. No status changes, no new seed
	// rows. Capture pre-run contents and diff.
	preSidecar := readBytes(t, sidecarPath)
	preSeed := readBytes(t, seedPath)
	code2 := runWithCapturedStdout(t, []string{"-no-db", "-seed", seedPath, "-group", "Test, May 2026", sidecarPath}, stdoutPath)
	if code2 != exitOK {
		t.Fatalf("second run exit: got %d, want %d", code2, exitOK)
	}
	if string(readBytes(t, sidecarPath)) != string(preSidecar) {
		t.Errorf("sidecar changed on second run — expected no-op")
	}
	if string(readBytes(t, seedPath)) != string(preSeed) {
		t.Errorf("seed file changed on second run — expected no-op")
	}
}

func writeFixtureSidecar(t *testing.T, path string, records []Record) {
	t.Helper()
	if err := writeSidecar(path, records); err != nil {
		t.Fatalf("write fixture sidecar: %v", err)
	}
}

func readFixtureSidecar(t *testing.T, path string) []Record {
	t.Helper()
	recs, err := readSidecar(path)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	return recs
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	return string(readBytes(t, path))
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// runWithCapturedStdout invokes run() with stdout captured in-memory and
// flushed to a file so callers can read the JSON summary. Stderr is
// discarded to keep `go test -v` output legible.
func runWithCapturedStdout(t *testing.T, args []string, stdoutPath string) int {
	t.Helper()
	var stdout bytes.Buffer
	code := run(args, &stdout, io.Discard)
	if err := os.WriteFile(stdoutPath, stdout.Bytes(), 0o644); err != nil {
		t.Fatalf("write captured stdout: %v", err)
	}
	return code
}

// TestRun_PreconditionMissingExits2 exercises the run()-level precondition
// path: a record missing url drives exit code 2 and the summary reports
// the record's rank under precondition_failures. Validates Fix #4's split
// between per-record annotator gaps (exit 2) and the in_progress_remaining
// counter (which counts all unfinalized records, including precondition
// ones).
func TestRun_PreconditionMissingExits2(t *testing.T) {
	dir := t.TempDir()
	sidecarPath := filepath.Join(dir, "candidates.jsonl")
	seedPath := filepath.Join(dir, "companies.sql")

	if err := os.WriteFile(seedPath, []byte(`INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Existing', 'greenhouse', 'existing', 'AI')
ON CONFLICT (ats, board_token) DO NOTHING;
`), 0o644); err != nil {
		t.Fatalf("seed setup: %v", err)
	}
	// One record missing url — the precondition gap that drives exit 2.
	records := []Record{
		{Rank: 42, Name: "NoURLCo"},
	}
	writeFixtureSidecar(t, sidecarPath, records)

	stdoutPath := filepath.Join(dir, "stdout.json")
	code := runWithCapturedStdout(t, []string{"-no-db", "-seed", seedPath, "-group", "Test", sidecarPath}, stdoutPath)
	if code != exitPreconditionMissing {
		t.Fatalf("exit code: got %d, want %d", code, exitPreconditionMissing)
	}

	var sum summary
	if err := json.Unmarshal(readBytes(t, stdoutPath), &sum); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if len(sum.PreconditionFailures) != 1 || sum.PreconditionFailures[0] != 42 {
		t.Errorf("PreconditionFailures: got %v, want [42]", sum.PreconditionFailures)
	}
	if sum.InProgressRemaining != 1 {
		t.Errorf("InProgressRemaining: got %d, want 1 (record still in-progress)", sum.InProgressRemaining)
	}
	if sum.Verified != 0 {
		t.Errorf("Verified: got %d, want 0", sum.Verified)
	}
}
