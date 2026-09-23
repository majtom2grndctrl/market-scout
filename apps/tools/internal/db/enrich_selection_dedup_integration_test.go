//go:build integration

// Integration tests for the dedup half of the enrichment selection queries.
// Require a live Postgres instance and DATABASE_URL env var.
// Run with (from apps/tools/): go test -tags=integration ./internal/db/...
package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

// dedupFixture is one posting to seed: an offset from a base timestamp for
// first_seen_at ordering, plus the snapshot columns the dedup edges read.
// nullBody seeds a NULL description_text; those rows carry the marker in their
// title instead, so the focus prefilter still reaches them and their absence
// from a result is attributable to the IS NOT NULL filter alone.
type dedupFixture struct {
	label       string
	minutes     int
	title       string
	requisition sql.NullString
	body        string
	nullBody    bool
}

// seedDedupCompany creates a throwaway company keyed by boardToken, which the
// (ats, board_token) unique constraint makes the isolation handle: a test that
// needs two companies passes two tokens.
func seedDedupCompany(t *testing.T, conn *sql.DB, boardToken string) int64 {
	t.Helper()

	var companyID int64
	if err := conn.QueryRowContext(t.Context(), `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, 'greenhouse', $2)
		RETURNING id
	`, "Market Scout Dedup Test", boardToken).Scan(&companyID); err != nil {
		t.Fatalf("insert company: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanupCtx,
			`DELETE FROM posting_snapshots WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = $1)`, companyID); err != nil {
			t.Logf("cleanup posting_snapshots: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM job_postings WHERE company_id = $1`, companyID); err != nil {
			t.Logf("cleanup job_postings: %v", err)
		}
		if _, err := conn.ExecContext(cleanupCtx, `DELETE FROM companies WHERE id = $1`, companyID); err != nil {
			t.Logf("cleanup companies: %v", err)
		}
	})

	return companyID
}

// seedDedupFixtures inserts one posting-plus-snapshot per fixture under
// companyID and returns the posting ids in fixture order. Every description
// embeds marker so a test can isolate its own rows from the live backlog via
// the focus prefilter — selection has no company parameter. The marker is
// appended verbatim, so two fixtures sharing a body still share byte-identical
// description text and so still share an md5.
func seedDedupFixtures(t *testing.T, conn *sql.DB, companyID int64, marker string, fixtures []dedupFixture) []int64 {
	t.Helper()
	ctx := t.Context()

	base := time.Now().UTC().Add(-24 * time.Hour)
	ids := make([]int64, 0, len(fixtures))
	for _, f := range fixtures {
		var postingID int64
		if err := conn.QueryRowContext(ctx, `
			INSERT INTO job_postings (company_id, source_type, source_url, first_seen_at)
			VALUES ($1, 'ats', $2, $3)
			RETURNING id
		`, companyID, "https://example.com/jobs/"+marker+"/"+f.label,
			base.Add(time.Duration(f.minutes)*time.Minute)).Scan(&postingID); err != nil {
			t.Fatalf("insert job_posting %s: %v", f.label, err)
		}

		title := f.title
		var body sql.NullString
		if f.nullBody {
			title = f.title + " " + marker
		} else {
			body = sql.NullString{String: f.body + " " + marker, Valid: true}
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO posting_snapshots (job_posting_id, fetched_at, title, raw_data, description_text, requisition_key)
			VALUES ($1, now(), $2, '{}'::jsonb, $3, $4)
		`, postingID, title, body, f.requisition); err != nil {
			t.Fatalf("insert posting_snapshot %s: %v", f.label, err)
		}
		ids = append(ids, postingID)
	}

	return ids
}

// seedDedupPostings is the single-company case: one throwaway company holding
// every fixture, with the marker doing double duty as board token and focus
// needle.
func seedDedupPostings(t *testing.T, conn *sql.DB, marker string, fixtures []dedupFixture) []int64 {
	t.Helper()
	return seedDedupFixtures(t, conn, seedDedupCompany(t, conn, marker), marker, fixtures)
}

func openDedupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.PingContext(t.Context()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return conn
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

// dedupUnits runs the deduped selection over one marker's rows and returns the
// per-posting dedup key and representative flag.
func dedupUnits(t *testing.T, conn *sql.DB, marker string, wantRows int) (map[int64]string, map[int64]bool) {
	t.Helper()

	rows, err := db.New(conn).ListUnclassifiedPostings(t.Context(), db.ListUnclassifiedPostingsParams{
		Focus:       marker,
		RowLimit:    100,
		Dedup:       true,
		NewestFirst: false,
	})
	if err != nil {
		t.Fatalf("ListUnclassifiedPostings: %v", err)
	}
	if len(rows) != wantRows {
		t.Fatalf("len(rows) = %d, want %d", len(rows), wantRows)
	}

	keyOf := map[int64]string{}
	repOf := map[int64]bool{}
	for _, r := range rows {
		keyOf[r.PostingID] = r.DedupKey
		repOf[r.PostingID] = r.IsRepresentative
	}
	return keyOf, repOf
}

// assertReps checks the representative flag for each posting in id order; want
// is parallel to ids. The representative is the lowest posting id in a unit and
// inserts are sequential, so the first fixture of each unit holds it.
func assertReps(t *testing.T, repOf map[int64]bool, ids []int64, want []bool) {
	t.Helper()
	for i, id := range ids {
		if repOf[id] != want[i] {
			t.Errorf("posting %d (fixture %d) is_representative = %v, want %v", id, i, repOf[id], want[i])
		}
	}
}

// The requisition edge is gated on a matching normalized title. At Greenhouse
// the requisition key names a job family, not a posting (Ashby carries no
// requisition key at all), so an ungated key merged four distinct Stripe Staff SWE roles into one
// classification. Normalization is what makes the gate usable: titles that
// differ only by case or runs of whitespace are the same title.
func TestListUnclassifiedPostings_DedupRequisitionEdgeRequiresMatchingTitle(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "msdedupreq" + time.Now().UTC().Format("20060102T150405.000000000")

	ids := seedDedupPostings(t, conn, marker, []dedupFixture{
		// One requisition key spanning two genuinely different jobs.
		{label: "a", minutes: 0, title: "Staff Software Engineer", requisition: nullStr("REQ-FAMILY"), body: "swe body"},
		{label: "b", minutes: 1, title: "Product Marketing Manager", requisition: nullStr("REQ-FAMILY"), body: "pmm body"},
		// One requisition, one title modulo case and whitespace, description
		// edited between fetches.
		{label: "c", minutes: 2, title: "Data Scientist", requisition: nullStr("REQ-TITLE"), body: "ds body"},
		{label: "d", minutes: 3, title: "  data   SCIENTIST ", requisition: nullStr("REQ-TITLE"), body: "ds body revised"},
	})

	keyOf, repOf := dedupUnits(t, conn, marker, 4)

	if keyOf[ids[0]] == keyOf[ids[1]] {
		t.Errorf("one requisition key merged two different titles into unit %q; the key names a job family, not a posting", keyOf[ids[0]])
	}
	if keyOf[ids[2]] != keyOf[ids[3]] {
		t.Errorf("same requisition key and same normalized title did not merge: %q vs %q", keyOf[ids[2]], keyOf[ids[3]])
	}
	assertReps(t, repOf, ids, []bool{true, true, true, false})

	// The forced variant carries the same dedup body; a drift between the two
	// would show up here as different grouping under identical fixtures.
	forced, err := db.New(conn).ListUnclassifiedPostingsForced(t.Context(), db.ListUnclassifiedPostingsForcedParams{
		Focus: marker, RowLimit: 100, Dedup: true,
	})
	if err != nil {
		t.Fatalf("ListUnclassifiedPostingsForced: %v", err)
	}
	forcedKeys := map[int64]string{}
	for _, r := range forced {
		forcedKeys[r.PostingID] = r.DedupKey
	}
	for id, want := range keyOf {
		if forcedKeys[id] != want {
			t.Errorf("forced variant grouped posting %d as %q, want %q", id, forcedKeys[id], want)
		}
	}
}

// Identical description text is the strong edge: it merges regardless of what
// the board says the requisition is, because identical text classifies
// identically. The same title over different text stays separate — that is
// usually a genuinely different job.
func TestListUnclassifiedPostings_DedupTextEdgeMergesAcrossRequisitionKeys(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "msdeduptext" + time.Now().UTC().Format("20060102T150405.000000000")

	ids := seedDedupPostings(t, conn, marker, []dedupFixture{
		// One job listed once per location: distinct requisition keys, byte-identical text.
		{label: "a", minutes: 0, title: "Account Executive", requisition: nullStr("REQ-X"), body: "shared body"},
		{label: "b", minutes: 1, title: "Account Executive", requisition: nullStr("REQ-Y"), body: "shared body"},
		// Same title, different text.
		{label: "c", minutes: 2, title: "Account Executive", requisition: nullStr("REQ-Z"), body: "other body"},
	})

	keyOf, repOf := dedupUnits(t, conn, marker, 3)

	if keyOf[ids[0]] != keyOf[ids[1]] {
		t.Errorf("identical description text under different requisition keys did not merge: %q vs %q", keyOf[ids[0]], keyOf[ids[1]])
	}
	if keyOf[ids[0]] == keyOf[ids[2]] {
		t.Errorf("same title with different text merged into unit %q, want separate units", keyOf[ids[2]])
	}
	assertReps(t, repOf, ids, []bool{true, false, true})
}

// Work units are connected components, not groups under one scalar key: the two
// edges chain. A shares text with B, B shares a gated requisition key with C, so
// all three are one job even though A and C have neither edge between them.
func TestListUnclassifiedPostings_DedupWalksTransitiveComponents(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "msdedupchain" + time.Now().UTC().Format("20060102T150405.000000000")

	ids := seedDedupPostings(t, conn, marker, []dedupFixture{
		{label: "a", minutes: 0, title: "Solutions Engineer", body: "chained body"},
		{label: "b", minutes: 1, title: "Forward Deployed Engineer", requisition: nullStr("REQ-CHAIN"), body: "chained body"},
		{label: "c", minutes: 2, title: "forward  deployed  engineer", requisition: nullStr("REQ-CHAIN"), body: "edited body"},
	})

	keyOf, repOf := dedupUnits(t, conn, marker, 3)

	if keyOf[ids[0]] != keyOf[ids[1]] || keyOf[ids[1]] != keyOf[ids[2]] {
		t.Errorf("transitive closure did not collapse the chain: %q, %q, %q", keyOf[ids[0]], keyOf[ids[1]], keyOf[ids[2]])
	}
	assertReps(t, repOf, ids, []bool{true, false, false})
}

// Both edges are scoped within one company. Two boards can publish the same
// boilerplate-heavy text or reuse a requisition-key format; neither makes one
// company's posting classifiable from another company's description.
func TestListUnclassifiedPostings_DedupScopesEdgesWithinOneCompany(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "msdedupscope" + time.Now().UTC().Format("20060102T150405.000000000")

	fixtures := []dedupFixture{
		{label: "a", minutes: 0, title: "Security Engineer", requisition: nullStr("REQ-SHARED"), body: "same body"},
	}
	first := seedDedupFixtures(t, conn, seedDedupCompany(t, conn, marker+"-one"), marker, fixtures)
	second := seedDedupFixtures(t, conn, seedDedupCompany(t, conn, marker+"-two"), marker, fixtures)

	keyOf, repOf := dedupUnits(t, conn, marker, 2)

	if keyOf[first[0]] == keyOf[second[0]] {
		t.Errorf("postings at two companies merged into unit %q; edges are company-scoped", keyOf[first[0]])
	}
	assertReps(t, repOf, []int64{first[0], second[0]}, []bool{true, true})
}

// Postings whose latest snapshot has no description_text are filtered out
// before any edge is built. Hashing coalesce(description_text, ”) instead
// would give every one of them the same hash and collapse a company's whole
// description-less backlog into a single work unit.
func TestListUnclassifiedPostings_DedupExcludesDescriptionlessPostings(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "msdedupnull" + time.Now().UTC().Format("20060102T150405.000000000")

	ids := seedDedupPostings(t, conn, marker, []dedupFixture{
		// Marker rides in the title here, so the focus prefilter reaches these
		// rows and their absence is the IS NOT NULL filter's doing.
		{label: "a", minutes: 0, title: "Recruiting Coordinator", requisition: nullStr("REQ-NULL"), nullBody: true},
		{label: "b", minutes: 1, title: "Recruiting Coordinator", requisition: nullStr("REQ-NULL"), nullBody: true},
		{label: "c", minutes: 2, title: "Recruiting Coordinator", requisition: nullStr("REQ-NULL"), body: "real body"},
	})

	keyOf, _ := dedupUnits(t, conn, marker, 1)

	if _, ok := keyOf[ids[2]]; !ok {
		t.Fatalf("the described posting %d was not selected", ids[2])
	}
	for _, id := range ids[:2] {
		if key, ok := keyOf[id]; ok {
			t.Errorf("description-less posting %d was selected into unit %q", id, key)
		}
	}
}

// The row limit bounds work units, not postings, and a unit is never split
// across it. A split unit would be classified in two halves on two runs,
// independently — the inconsistency dedup exists to prevent.
func TestListUnclassifiedPostings_DedupLimitBoundsUnitsAndKeepsSiblings(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "msdeduplimit" + time.Now().UTC().Format("20060102T150405.000000000")

	seedDedupPostings(t, conn, marker, []dedupFixture{
		{label: "a", minutes: 0, title: "Solutions Architect", requisition: nullStr("REQ-A"), body: "one"},
		{label: "b", minutes: 1, title: "Solutions Architect", requisition: nullStr("REQ-A"), body: "one edited"},
		{label: "c", minutes: 2, title: "Solutions Architect", requisition: nullStr("REQ-A"), body: "one edited twice"},
		{label: "d", minutes: 3, title: "Data Engineer", requisition: nullStr("REQ-B"), body: "two"},
		{label: "e", minutes: 4, title: "Platform Engineer", requisition: nullStr("REQ-C"), body: "three"},
	})

	queries := db.New(conn)

	// Two units requested: the three-posting unit plus the next one, so five
	// postings collapse to two units and four rows come back.
	deduped, err := queries.ListUnclassifiedPostings(t.Context(), db.ListUnclassifiedPostingsParams{
		Focus: marker, RowLimit: 2, Dedup: true,
	})
	if err != nil {
		t.Fatalf("deduped select: %v", err)
	}
	reps := 0
	for _, r := range deduped {
		if r.IsRepresentative {
			reps++
		}
	}
	if reps != 2 {
		t.Errorf("representatives = %d, want 2 (row_limit bounds work units)", reps)
	}
	if len(deduped) != 4 {
		t.Errorf("len(rows) = %d, want 4 — the first unit's three postings must all ride along past the limit", len(deduped))
	}

	// Same limit without dedup keeps the original row-per-posting contract that
	// cmd/batch-enrich depends on: no edges are built, so every posting is a
	// unit of one and the limit counts postings again.
	plain, err := queries.ListUnclassifiedPostings(t.Context(), db.ListUnclassifiedPostingsParams{
		Focus: marker, RowLimit: 2, Dedup: false,
	})
	if err != nil {
		t.Fatalf("plain select: %v", err)
	}
	if len(plain) != 2 {
		t.Errorf("len(rows) = %d with dedup off, want exactly the row limit of 2", len(plain))
	}
	keys := map[string]bool{}
	for _, r := range plain {
		if !r.IsRepresentative {
			t.Errorf("posting %d is_representative = false with dedup off; every posting is its own unit", r.PostingID)
		}
		keys[r.DedupKey] = true
	}
	if len(keys) != len(plain) {
		t.Errorf("distinct dedup keys = %d over %d rows; with dedup off no two postings share a unit", len(keys), len(plain))
	}
}

// A wave has to spread across the watchlist even when one company could fill
// it on its own. The corpus is ~43% OpenAI/Stripe/Anthropic, and before the cap
// a 50-unit wave on 2026-09-21 returned 58 postings from one company. The
// ordering is what spreads (every company's newest unit, then every company's
// second-newest); @max_per_company is the ceiling that stops a large board
// refilling the tail of a big wave.
func TestListUnclassifiedPostings_CapsUnitsPerCompanyAndCyclesAcross(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "mscap" + time.Now().UTC().Format("20060102T150405.000000000")

	// A giant with six units and a small board with one. Minutes ascend, so
	// under newest-first the giant's "g6" is the newest posting overall.
	giant := seedDedupCompany(t, conn, marker+"-giant")
	seedDedupFixtures(t, conn, giant, marker, []dedupFixture{
		{label: "g1", minutes: 0, title: "Giant One", body: "g one"},
		{label: "g2", minutes: 1, title: "Giant Two", body: "g two"},
		{label: "g3", minutes: 2, title: "Giant Three", body: "g three"},
		{label: "g4", minutes: 3, title: "Giant Four", body: "g four"},
		{label: "g5", minutes: 4, title: "Giant Five", body: "g five"},
		{label: "g6", minutes: 5, title: "Giant Six", body: "g six"},
	})
	small := seedDedupCompany(t, conn, marker+"-small")
	seedDedupFixtures(t, conn, small, marker, []dedupFixture{
		{label: "s1", minutes: -60, title: "Small One", body: "s one"},
	})

	queries := db.New(conn)

	perCompany := func(rows []db.ListUnclassifiedPostingsRow) map[int64]int {
		counts := map[int64]int{}
		for _, r := range rows {
			counts[r.CompanyID]++
		}
		return counts
	}

	// Uncapped (@max_per_company <= 0) and asking for every unit: the giant
	// contributes all six. This is the shape the cap exists to prevent, and it
	// is also what the pre-cap query did.
	uncapped, err := queries.ListUnclassifiedPostings(t.Context(), db.ListUnclassifiedPostingsParams{
		Focus: marker, RowLimit: 100, Dedup: true, NewestFirst: true, MaxPerCompany: 0,
	})
	if err != nil {
		t.Fatalf("uncapped select: %v", err)
	}
	if got := perCompany(uncapped)[giant]; got != 6 {
		t.Fatalf("uncapped giant units = %d, want 6", got)
	}

	// Capped at two: the giant is held to its two newest units, and the small
	// company's only unit survives even though six giant units are newer.
	capped, err := queries.ListUnclassifiedPostings(t.Context(), db.ListUnclassifiedPostingsParams{
		Focus: marker, RowLimit: 100, Dedup: true, NewestFirst: true, MaxPerCompany: 2,
	})
	if err != nil {
		t.Fatalf("capped select: %v", err)
	}
	counts := perCompany(capped)
	if counts[giant] != 2 {
		t.Errorf("capped giant units = %d, want 2", counts[giant])
	}
	if counts[small] != 1 {
		t.Errorf("capped small-company units = %d, want 1", counts[small])
	}

	// Ordering cycles across companies before recency. With a row limit of 2
	// the wave takes one unit from each company, not the giant's two newest —
	// which is what makes a wave representative rather than merely bounded.
	wave, err := queries.ListUnclassifiedPostings(t.Context(), db.ListUnclassifiedPostingsParams{
		Focus: marker, RowLimit: 2, Dedup: true, NewestFirst: true, MaxPerCompany: 2,
	})
	if err != nil {
		t.Fatalf("wave select: %v", err)
	}
	if len(wave) != 2 {
		t.Fatalf("len(wave) = %d, want 2", len(wave))
	}
	waveCounts := perCompany(wave)
	if waveCounts[giant] != 1 || waveCounts[small] != 1 {
		t.Errorf("wave company spread = %v, want one unit each from %d and %d", waveCounts, giant, small)
	}
	// The giant's contribution is still its newest unit — the cap changes how
	// many a company gets, never which ones.
	for _, r := range wave {
		if r.CompanyID == giant && r.Title.String != "Giant Six" {
			t.Errorf("giant unit in wave = %q, want the newest (\"Giant Six\")", r.Title.String)
		}
	}
}

// Oldest-first stays reachable for a deliberate backlog drain, and the cap
// applies to it the same way: a capped company gives up its oldest units, not
// its newest ones.
func TestListUnclassifiedPostings_CapFollowsTheSortDirection(t *testing.T) {
	conn := openDedupTestDB(t)
	marker := "mscapsort" + time.Now().UTC().Format("20060102T150405.000000000")

	seedDedupPostings(t, conn, marker, []dedupFixture{
		{label: "a", minutes: 0, title: "Oldest", body: "a"},
		{label: "b", minutes: 1, title: "Middle", body: "b"},
		{label: "c", minutes: 2, title: "Newest", body: "c"},
	})

	queries := db.New(conn)

	for _, tc := range []struct {
		name        string
		newestFirst bool
		wantTitle   string
	}{
		{name: "newest first keeps the newest unit", newestFirst: true, wantTitle: "Newest"},
		{name: "oldest first keeps the oldest unit", newestFirst: false, wantTitle: "Oldest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := queries.ListUnclassifiedPostings(t.Context(), db.ListUnclassifiedPostingsParams{
				Focus: marker, RowLimit: 100, Dedup: true, NewestFirst: tc.newestFirst, MaxPerCompany: 1,
			})
			if err != nil {
				t.Fatalf("select: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("len(rows) = %d, want 1 under a cap of one unit", len(rows))
			}
			if rows[0].Title.String != tc.wantTitle {
				t.Errorf("selected title = %q, want %q", rows[0].Title.String, tc.wantTitle)
			}
		})
	}
}
